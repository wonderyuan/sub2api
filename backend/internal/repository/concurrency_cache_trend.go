package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	userConcurrencyTrendKeyPrefix = "ops:concurrency:trend:minute:"
	userConcurrencyTrendTTL       = 70 * time.Minute
)

var mergeUserConcurrencyTrendScript = redis.NewScript(`
	for i = 2, #ARGV, 2 do
		local field = ARGV[i]
		local value = tonumber(ARGV[i + 1]) or 0
		local current = redis.call('HGET', KEYS[1], field)
		if current == false or value > tonumber(current) then
			redis.call('HSET', KEYS[1], field, value)
		end
	end
	redis.call('EXPIRE', KEYS[1], ARGV[1])
	return 1
`)

func runScriptInt64Triple(ctx context.Context, rdb *redis.Client, script *redis.Script, keys []string, args ...any) (int64, int64, int64, error) {
	raw, err := script.Run(ctx, rdb, keys, args...).Result()
	if err != nil {
		return 0, 0, 0, err
	}
	first, err := redisScriptInt64At(raw, 0)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse script value 0: %w", err)
	}
	second, err := redisScriptInt64At(raw, 1)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse script value 1: %w", err)
	}
	third, err := redisScriptInt64At(raw, 2)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse script value 2: %w", err)
	}
	return first, second, third, nil
}

func userConcurrencyTrendKey(bucketStart time.Time) string {
	return userConcurrencyTrendKeyPrefix + strconv.FormatInt(bucketStart.UTC().Truncate(time.Minute).Unix(), 10)
}

func (c *concurrencyCache) GetActiveUserLoads(ctx context.Context) (map[int64]*service.UserLoadInfo, error) {
	if c == nil || c.rdb == nil {
		return map[int64]*service.UserLoadInfo{}, nil
	}
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return nil, err
	}
	members, err := c.rdb.ZRange(ctx, userActiveIndexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("read active user index: %w", err)
	}
	loads, staleMembers, err := c.readIndexLoads(ctx, userSlotIndex, members, now)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]*service.UserLoadInfo, len(loads))
	for _, load := range loads {
		if load.slotCount <= 0 && load.waitCount <= 0 {
			staleMembers = append(staleMembers, load.member)
			continue
		}
		result[load.id] = &service.UserLoadInfo{
			UserID:             load.id,
			CurrentConcurrency: max(load.slotCount, 0),
			WaitingCount:       max(load.waitCount, 0),
		}
	}
	c.removeActiveIndexMembers(ctx, userActiveIndexKey, staleMembers)
	return result, nil
}

func (c *concurrencyCache) MergeUserConcurrencyTrend(
	ctx context.Context,
	bucketStart time.Time,
	users map[int64]service.ConcurrencyPeak,
	system service.ConcurrencyPeak,
) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	args := make([]any, 0, 8+len(users)*6)
	args = append(args, int(userConcurrencyTrendTTL.Seconds()),
		"s:a", system.PeakInUse,
		"s:w", system.PeakWaiting,
		"s:d", system.PeakDemand,
	)
	for userID, peak := range users {
		prefix := "u:" + strconv.FormatInt(userID, 10) + ":"
		args = append(args,
			prefix+"a", peak.PeakInUse,
			prefix+"w", peak.PeakWaiting,
			prefix+"d", peak.PeakDemand,
		)
	}
	_, err := mergeUserConcurrencyTrendScript.Run(ctx, c.rdb, []string{userConcurrencyTrendKey(bucketStart)}, args...).Result()
	return err
}

func (c *concurrencyCache) GetUserConcurrencyTrend(ctx context.Context, start, end time.Time) (*service.UserConcurrencyTrend, error) {
	start = start.UTC().Truncate(time.Minute)
	end = end.UTC().Truncate(time.Minute)
	if end.Before(start) {
		return nil, fmt.Errorf("invalid concurrency trend range")
	}

	pipe := c.rdb.Pipeline()
	type bucketCommand struct {
		bucket time.Time
		cmd    *redis.MapStringStringCmd
	}
	commands := make([]bucketCommand, 0, int(end.Sub(start)/time.Minute)+1)
	for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Minute) {
		commands = append(commands, bucketCommand{
			bucket: bucket,
			cmd:    pipe.HGetAll(ctx, userConcurrencyTrendKey(bucket)),
		})
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("read user concurrency trend: %w", err)
	}

	points := make([]service.UserConcurrencyTrendPoint, 0, len(commands))
	for _, command := range commands {
		point := service.UserConcurrencyTrendPoint{
			BucketStart: command.bucket,
			Users:       make(map[int64]service.ConcurrencyPeak),
		}
		for field, raw := range command.cmd.Val() {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				continue
			}
			switch field {
			case "s:a":
				point.System.PeakInUse = value
			case "s:w":
				point.System.PeakWaiting = value
			case "s:d":
				point.System.PeakDemand = value
			default:
				parts := strings.Split(field, ":")
				if len(parts) != 3 || parts[0] != "u" {
					continue
				}
				userID, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil || userID <= 0 {
					continue
				}
				peak := point.Users[userID]
				switch parts[2] {
				case "a":
					peak.PeakInUse = value
				case "w":
					peak.PeakWaiting = value
				case "d":
					peak.PeakDemand = value
				default:
					continue
				}
				point.Users[userID] = peak
			}
		}
		points = append(points, point)
	}

	return &service.UserConcurrencyTrend{
		StartTime: start,
		EndTime:   end,
		Bucket:    "minute",
		Points:    points,
	}, nil
}
