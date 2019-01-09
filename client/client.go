package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func configure() error {
	viper.SetConfigFile("config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	logLevel, err := logrus.ParseLevel(viper.GetString("log_level"))
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)

	return nil
}

func calc(tasksWriter *kafka.Writer, body string) error {
	if err := tasksWriter.WriteMessages(context.Background(),
		kafka.Message{
			Key:   uuid.NewV1().Bytes(),
			Value: []byte(body),
		}); err != nil {
		return fmt.Errorf("Failed to publish message: %v", err)
	}
	return nil
}

func startPrintingResults(resultsReader *kafka.Reader) {
	for {
		msg, err := resultsReader.ReadMessage(context.Background())
		if err != nil {
			logrus.Errorf("failed to ReadMessage: %v", err)
			continue
		}
		msg.Value = bytes.TrimSpace(msg.Value)
		logrus.Infof("Result: %s", string(msg.Value))
	}
}

func main() {
	if err := configure(); err != nil {
		logrus.Fatal(err)
	}

	kafkaBrokers := viper.GetStringSlice("brokers")
	kafkaTasks := viper.GetString("kafka_tasks")
	kafkaResults := viper.GetString("kafka_results")

	kafkaTasksWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      kafkaBrokers,
		Topic:        kafkaTasks,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: 1,
	})
	kafkaResultsReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kafkaBrokers,
		Topic:          kafkaResults,
		MinBytes:       10e3,        // 10KB
		MaxBytes:       10e6,        // 10MB
		CommitInterval: time.Second, // flushes commits to Kafka every second
	})

	go startPrintingResults(kafkaResultsReader)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Command: ")
		text, _ := reader.ReadString('\n')
		text = strings.Replace(text, "  ", " ", -1)
		parts := strings.Split(text, " ")
		if len(parts) == 0 {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(parts[0])) {
		case "CALC":
			if len(parts) != 2 {
				logrus.Errorln("Invalid command. Expected 1 argument: body")
				continue
			}

			body := parts[1]
			if err := calc(kafkaTasksWriter, body); err != nil {
				logrus.Errorln(err)
				continue
			}
		case "EXIT":
			logrus.Println("Exit OK.")
			return
		default:
			logrus.Errorln("Invalid command.")
		}
	}
}
