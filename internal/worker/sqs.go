package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

type SQSWorker struct {
	engine   *Engine
	client   *sqs.Client
	queueURL string
}

type SQSMessagePayload struct {
	MetaPRID string `json:"meta_pr_id"`
}

func NewSQSWorker(engine *Engine, queueURL string) (*SQSWorker, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := sqs.NewFromConfig(cfg)
	return &SQSWorker{
		engine:   engine,
		client:   client,
		queueURL: queueURL,
	}, nil
}

func (w *SQSWorker) Start(ctx context.Context) {
	log.Printf("[sqs-worker] polling from queue URL: %s", w.queueURL)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			w.poll(ctx)
		}
	}
}

func (w *SQSWorker) poll(ctx context.Context) {
	resp, err := w.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(w.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     10,
		VisibilityTimeout:   30,
	})
	if err != nil {
		log.Printf("[sqs-worker] receive message error: %v", err)
		time.Sleep(2 * time.Second)
		return
	}

	for _, msg := range resp.Messages {
		if err := w.processMessage(ctx, msg); err != nil {
			log.Printf("[sqs-worker] error processing message %s: %v", *msg.MessageId, err)
		} else {
			// Delete message from queue upon success
			_, err = w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(w.queueURL),
				ReceiptHandle: msg.ReceiptHandle,
			})
			if err != nil {
				log.Printf("[sqs-worker] failed to delete message: %v", err)
			}
		}
	}
}

func (w *SQSWorker) processMessage(ctx context.Context, msg types.Message) error {
	var payload SQSMessagePayload
	if err := json.Unmarshal([]byte(*msg.Body), &payload); err != nil {
		return err
	}

	prID, err := uuid.Parse(payload.MetaPRID)
	if err != nil {
		return err
	}

	log.Printf("[sqs-worker] triggering cascade merge for Meta PR ID: %s", prID)
	return w.engine.ExecuteCascadeMerge(ctx, prID)
}
