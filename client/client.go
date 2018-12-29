package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
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

func calc(channel *amqp.Channel, exchange, key, body string) error {
	if err := channel.Publish(exchange, key, false, false, amqp.Publishing{
		ContentType: "text/plain",
		MessageId:   uuid.NewV1().String(),
		Body:        []byte(body),
	}); err != nil {
		return fmt.Errorf("Failed to publish message: %v", err)
	}

	return nil
}

func startPrintingResults(channel *amqp.Channel, queue string) error {
	resultsC, err := channel.Consume(queue, "", true, true, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for {
			msg, ok := <-resultsC
			if !ok {
				logrus.Errorln("Channel resultC is closed")
				os.Exit(1)
			}
			logrus.Infof("Result: %s", string(msg.Body))
		}
	}()
	return nil
}

func main() {
	if err := configure(); err != nil {
		logrus.Fatal(err)
	}

	rmqDialString := viper.GetString("rabbitmq")
	rmqExchange := viper.GetString("rmq_exchange")
	rmqKeyTasks := viper.GetString("rmq_key_tasks")
	rmqKeyResults := viper.GetString("rmq_key_results")

	conn, err := amqp.Dial(rmqDialString)
	if err != nil {
		logrus.Fatal(err)
	}

	ch, err := conn.Channel()
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		if err := ch.Close(); err != nil {
			logrus.Errorf("Channel close error: %v", err)
		}
	}()

	if err := startPrintingResults(ch, rmqKeyResults); err != nil {
		logrus.Fatal(err)
	}

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
			if err := calc(ch, rmqExchange, rmqKeyTasks, body); err != nil {
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
