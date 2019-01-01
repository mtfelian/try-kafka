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
	//  Topic              string
	//    NumPartitions      int
	//    ReplicationFactor  int
	//    ReplicaAssignments []ReplicaAssignment
	//    ConfigEntries      []ConfigEntry
	return conn.CreateTopics(
		kafka.TopicConfig{Topic: qIn, NumPartitions: 2},
		kafka.TopicConfig{Topic: qOut, NumPartitions: 2},
	)
}

func initialize(conn *kafka.Conn, qIn, qOut string) error {
	cleanup(conn, qIn, qOut)
	return create(conn, qIn, qOut)
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
		logrus.Fatal("E0", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logrus.Errorf("Channel close error: %v", err)
		}
	}()

	fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>INIT BEG")
	if err := initialize(conn, kafkaTasks, kafkaResults); err != nil {
		logrus.Fatal("E1", err)
	}
	fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>INIT END")

	kafkaResultsWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  kafkaBrokers,
		Topic:    kafkaResults,
		Balancer: &kafka.LeastBytes{},
	})
	kafkaResultsReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kafkaBrokers,
		Topic:          kafkaTasks,
		MinBytes:       10e3,        // 10KB
		MaxBytes:       10e6,        // 10MB
		CommitInterval: time.Second, // flushes commits to Kafka every second
	})

	startPerformTasks(kafkaResultsWriter, kafkaResultsReader)
}
