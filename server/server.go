package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Knetic/govaluate"
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

func cleanup(conn *kafka.Conn, qIn, qOut string) {
	if err := conn.DeleteTopics(qOut, qIn); err != nil {
		logrus.Errorf("failed to DeleteTopics(): %v", err)
	}
}

func create(conn *kafka.Conn, qIn, qOut string) error {
	return conn.CreateTopics(
		kafka.TopicConfig{Topic: qIn},
		kafka.TopicConfig{Topic: qOut},
	)
}

func initialize(conn *kafka.Conn, qIn, qOut string) error {
	cleanup(conn, qIn, qOut)
	if err := create(conn, qIn, qOut); err != nil {
		logrus.Errorf("failed to create(): %v", err)
	}
	return nil
}

// evaluate calculates the formula for the given val
func evaluate(formula string) (float64, error) {
	expr, err := govaluate.NewEvaluableExpression(formula)
	if err != nil {
		return 0, fmt.Errorf("failed to compile formula %s: %v", formula, err)
	}

	data := map[string]interface{}{}
	r, err := expr.Evaluate(data)
	if err != nil {
		return 0, fmt.Errorf("failed to evaluate formula %s (data: %#v): %v", formula, data, err)
	}

	rFloat64, ok := r.(float64)
	if !ok {
		return 0, fmt.Errorf("result is not float64 for (data: %#v), formula %s", data, formula)
	}
	return rFloat64, nil
}

type result struct {
	Key  []byte  `json:"key"`
	Data float64 `json:"data"`
	Err  string  `json:"err"`
	OK   bool    `json:"ok"`
}

func startPerformTasks(resultsWriter *kafka.Writer, tasksReader *kafka.Reader) {
	logrus.Infoln("Starting main loop...")
	for {
		msg, err := tasksReader.ReadMessage(context.Background())
		if err != nil {
			logrus.Errorf("failed to ReadMessage: %v", err)
			continue
		}
		msg.Value = bytes.TrimSpace(msg.Value)

		var resultStruct result
		r, err := evaluate(string(msg.Value))
		resultStruct = result{OK: true, Key: msg.Key, Data: r}
		if err != nil {
			resultStruct = result{Err: err.Error(), OK: false, Key: msg.Key}
		}

		resultJSON, err := json.Marshal(resultStruct)
		if err != nil {
			resultJSON = []byte(fmt.Sprintf(`{"ok":false,"id":%s,"err":%q}`, string(msg.Key), err.Error()))
		}
		logrus.Debugf("Publishing result... %q gives %q", string(msg.Value), string(resultJSON))
		if err := resultsWriter.WriteMessages(context.Background(),
			kafka.Message{
				Key:   uuid.NewV1().Bytes(),
				Value: resultJSON,
			}); err != nil {
			logrus.Errorf("failed to WriteMessages: %v", err)
			continue
		}
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

	kafkaBroker0 := kafkaBrokers[0]
	conn, err := kafka.Dial("tcp", kafkaBroker0)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logrus.Errorf("Channel close error: %v", err)
		}
	}()

	if err := initialize(conn, kafkaTasks, kafkaResults); err != nil {
		logrus.Fatal(err)
	}

	kafkaResultsWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      kafkaBrokers,
		Topic:        kafkaResults,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: 1,
	})
	kafkaTasksReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kafkaBrokers,
		Topic:          kafkaTasks,
		MinBytes:       10e3,
		MaxWait:        500 * time.Millisecond,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
	})
	defer func() {
		logrus.Infof("closing kafkaResultsWriter: %v", kafkaResultsWriter.Close())
		logrus.Infof("closing kafkaResultsReader: %v", kafkaTasksReader.Close())
	}()

	startPerformTasks(kafkaResultsWriter, kafkaTasksReader)
}
