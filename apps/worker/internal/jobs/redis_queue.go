package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisReservationIdle = 5 * time.Minute

type RedisQueue struct {
	client     *redis.Client
	stream     string
	group      string
	consumer   string
	retrySet   string
	deadStream string
	clock      Clock
}

func NewRedisQueue(ctx context.Context, config RedisConfig) (*RedisQueue, error) {
	if !config.Valid() {
		return nil, errors.New("complete Redis queue configuration is required")
	}
	options, err := redis.ParseURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client := redis.NewClient(options)
	if err = client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	if err = client.XGroupCreateMkStream(ctx, config.StreamName, config.GroupName, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		_ = client.Close()
		return nil, fmt.Errorf("create Redis consumer group: %w", err)
	}
	return &RedisQueue{
		client: client, stream: config.StreamName, group: config.GroupName, consumer: config.Consumer,
		retrySet: config.StreamName + ":retries", deadStream: config.StreamName + ":dead", clock: SystemClock{},
	}, nil
}

func (q *RedisQueue) Close() error { return q.client.Close() }

func (q *RedisQueue) ResultStatus(ctx context.Context, idempotencyKey string) (string, error) {
	status, err := q.client.Get(ctx, q.resultKey(idempotencyKey)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return status, err
}

func (q *RedisQueue) ResultStatuses(ctx context.Context, idempotencyKeys []string) (map[string]string, error) {
	result := make(map[string]string, len(idempotencyKeys))
	if len(idempotencyKeys) == 0 {
		return result, nil
	}
	keys := make([]string, len(idempotencyKeys))
	for index, key := range idempotencyKeys {
		keys[index] = q.resultKey(key)
	}
	values, err := q.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for index, value := range values {
		if status, ok := value.(string); ok {
			result[idempotencyKeys[index]] = status
		}
	}
	return result, nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, job Job) error {
	if err := validateJob(job); err != nil {
		return err
	}
	claimed, err := q.client.SetNX(ctx, q.dedupeKey(job.IdempotencyKey), job.ID, 14*24*time.Hour).Result()
	if err != nil || !claimed {
		return err
	}
	if err = q.add(ctx, job); err != nil {
		_ = q.client.Del(ctx, q.dedupeKey(job.IdempotencyKey)).Err()
		return err
	}
	return nil
}

func (q *RedisQueue) Reserve(ctx context.Context) (Job, error) {
	if err := q.promoteRetries(ctx); err != nil {
		return Job{}, err
	}
	claimed, _, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: q.stream, Group: q.group, Consumer: q.consumer, MinIdle: redisReservationIdle, Start: "0-0", Count: 1,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return Job{}, err
	}
	if len(claimed) > 0 {
		return q.decodeReserved(ctx, claimed[0])
	}
	streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: q.group, Consumer: q.consumer, Streams: []string{q.stream, ">"}, Count: 1, Block: 2 * time.Second,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return Job{}, ErrNoJobs
	}
	if err != nil {
		return Job{}, err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return Job{}, ErrNoJobs
	}
	return q.decodeReserved(ctx, streams[0].Messages[0])
}

func (q *RedisQueue) Succeed(ctx context.Context, job Job) error {
	if job.ReceiptID == "" {
		return errors.New("Redis job receipt is required")
	}
	pipe := q.client.TxPipeline()
	pipe.Set(ctx, q.resultKey(job.IdempotencyKey), "succeeded", 14*24*time.Hour)
	pipe.XAck(ctx, q.stream, q.group, job.ReceiptID)
	pipe.XDel(ctx, q.stream, job.ReceiptID)
	_, err := pipe.Exec(ctx)
	return err
}

func (q *RedisQueue) Fail(ctx context.Context, job Job, jobErr error) error {
	if job.ReceiptID == "" {
		return errors.New("Redis job receipt is required")
	}
	receiptID := job.ReceiptID
	job.Attempts++
	job.ReceiptID = ""
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	pipe := q.client.TxPipeline()
	if job.Attempts >= job.MaxAttempts {
		pipe.XAdd(ctx, &redis.XAddArgs{Stream: q.deadStream, Values: map[string]any{
			"job": string(payload), "error": safeJobError(jobErr), "failed_at": q.now().Format(time.RFC3339Nano),
		}})
		pipe.Set(ctx, q.resultKey(job.IdempotencyKey), "dead", 14*24*time.Hour)
	} else {
		job.AvailableAt = q.now().Add(backoff(job.Attempts))
		payload, err = json.Marshal(job)
		if err != nil {
			return err
		}
		pipe.ZAdd(ctx, q.retrySet, redis.Z{Score: float64(job.AvailableAt.UnixMilli()), Member: string(payload)})
	}
	pipe.XAck(ctx, q.stream, q.group, receiptID)
	pipe.XDel(ctx, q.stream, receiptID)
	_, err = pipe.Exec(ctx)
	return err
}

func (q *RedisQueue) add(ctx context.Context, job Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.client.XAdd(ctx, &redis.XAddArgs{Stream: q.stream, Values: map[string]any{"job": string(payload)}}).Err()
}

func (q *RedisQueue) decodeReserved(ctx context.Context, message redis.XMessage) (Job, error) {
	payload := fmt.Sprint(message.Values["job"])
	var job Job
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		_ = q.client.XAck(ctx, q.stream, q.group, message.ID).Err()
		return Job{}, fmt.Errorf("decode Redis job: %w", err)
	}
	if completed, err := q.client.Exists(ctx, q.resultKey(job.IdempotencyKey)).Result(); err != nil {
		return Job{}, err
	} else if completed > 0 {
		pipe := q.client.TxPipeline()
		pipe.XAck(ctx, q.stream, q.group, message.ID)
		pipe.XDel(ctx, q.stream, message.ID)
		_, _ = pipe.Exec(ctx)
		return Job{}, ErrNoJobs
	}
	job.ReceiptID = message.ID
	return job, nil
}

var promoteRetriesScript = redis.NewScript(`
local jobs = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 100)
for _, job in ipairs(jobs) do
  if redis.call('ZREM', KEYS[1], job) == 1 then
    redis.call('XADD', KEYS[2], '*', 'job', job)
  end
end
return #jobs
`)

func (q *RedisQueue) promoteRetries(ctx context.Context) error {
	return promoteRetriesScript.Run(ctx, q.client, []string{q.retrySet, q.stream}, q.now().UnixMilli()).Err()
}

func (q *RedisQueue) now() time.Time {
	if q.clock == nil {
		return time.Now().UTC()
	}
	return q.clock.Now().UTC()
}

func (q *RedisQueue) dedupeKey(key string) string { return q.stream + ":dedupe:" + key }
func (q *RedisQueue) resultKey(key string) string { return q.stream + ":result:" + key }

func safeJobError(err error) string {
	if err == nil {
		return "unknown job failure"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
