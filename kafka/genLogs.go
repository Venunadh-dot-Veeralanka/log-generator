package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var client http.Client = http.Client{
	Timeout:   30 * time.Second,
	Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
}
var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

var count = 0

// Config ...
type Config struct {
	IP                string            `json:"ip"`
	KafkaTopics       []string          `json:"kafka_topics"`
	AuthToken         string            `json:"auth_token"`
	Tags              map[string]string `json:"tags"`
	ExtraTags         map[string]string `json:"extraJson"`
	MaxBulkCount      uint64            `json:"max_bulk_count"`
	MaxBulkSize       uint64            `json:"max_bulk_size"`
	LogsPerMin        uint64            `json:"logs_per_min"`
	FlushInterval     uint64            `json:"flush_interval"`
	SaveLogsToFile    string            `json:"save_logs_onto_file"`
	SendLargeJsonLogs string            `json:"send_json_logs_kafka"`
}

func getRandomLog(allLogs []string, config *Config) map[string]interface{} {

	rand.Seed(time.Now().UnixNano())
	indexToUse := rand.Intn(len(allLogs))

	logWithLevel := strings.SplitN(allLogs[indexToUse], ",", 2)

	record := make(map[string]interface{})
	record["level"] = strings.ToLower(strings.TrimSpace(strings.SplitN(logWithLevel[0], "=", 2)[1]))
	record["time"] = time.Now().Unix() * 1000

	for key, value := range config.Tags {
		record[key] = value
	}

	if config.SendLargeJsonLogs == "true" {
		for key, value := range config.ExtraTags {
			if key == "log_index" {
				count = count + 1
				record[key] = strconv.Itoa(count)
			} else {
				record[key] = strings.ReplaceAll(value, "", func(n int) string {
					b := make([]rune, n)
					for i := range b {
						b[i] = letters[rand.Intn(len(letters))]
					}
					return string(b)
				}(7))
			}
		}
	}

	message := strings.TrimSpace(strings.SplitN(logWithLevel[1], "=", 2)[1])
	message = strings.ReplaceAll(message, "$IP", fmt.Sprintf("%d.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256)))
	message = strings.ReplaceAll(message, "$INT", fmt.Sprintf("%d", rand.Intn(65535)+1))
	message = strings.ReplaceAll(message, "$STRING", func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		return string(b)
	}(10))

	record["message"] = message

	kafkaRecord := make(map[string]interface{})
	kafkaRecord["value"] = record

	return kafkaRecord
}

func sendToFile(logsToSave []map[string]interface{}, config *Config) {
	if config.SaveLogsToFile == "true" {
		kafkaRecords := make(map[string]interface{})
		kafkaRecords["records"] = logsToSave

		kafkaData, err := json.Marshal(kafkaRecords)
		if err != nil {
			log.Print(err)
			return
		}

		file, err := os.OpenFile("jsonLogs.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
			return
		}
		if _, err := file.Write([]byte(kafkaData)); err != nil {
			log.Fatal(err)
			return
		}
		if err := file.Close(); err != nil {
			log.Fatal(err)
			return
		}
	}
}

func sendToKafka(logsToSend []map[string]interface{}, config *Config, topicName string) {

	kafkaURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(config.IP, "/"), topicName)
	kafkaRecords := make(map[string]interface{})
	kafkaRecords["records"] = logsToSend

	kafkaData, err := json.Marshal(kafkaRecords)
	if err != nil {
		log.Print(err)
		return
	}

	fmt.Printf("Sending %d logs / %d bytes to Kafka\n", len(logsToSend), len(kafkaData))

	req, err := http.NewRequest("POST", kafkaURL, bytes.NewBuffer(kafkaData))
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("Content-Type", "application/vnd.kafka.json.v2+json")
	req.Header.Set("Content-Type", "application/vnd.kafka.json.v2+json")
	req.Header.Set("Accept", "application/vnd.kafka.v2+json,application/json")
	req.Header.Set("Authorization", config.AuthToken)

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		fmt.Printf("Failed to send Kafka records due to code:%s,response:%v\n", res.Status, res.Body)
	}
}

func generateLogsForOneMinute(startTime time.Time, config *Config, allLogs []string, topicName string) {

	totalLogsToSend := int(config.LogsPerMin)
	sizeCheckCounter := 0

	for range time.Tick(time.Duration(config.FlushInterval) * time.Second) {

		logsToSendInThisFlush := int(math.Ceil((float64(config.LogsPerMin) / float64(60/config.FlushInterval))))
		logsToSend := make([]map[string]interface{}, 0)

		for {

			logsToSend = append(logsToSend, getRandomLog(allLogs, config))
			sendToFile(logsToSend, config)
			logsToSendInThisFlush--
			totalLogsToSend--
			sizeCheckCounter++

			if func() bool {

				if logsToSendInThisFlush <= 0 {
					return true
				}

				if len(logsToSend) >= int(config.MaxBulkCount) {
					return true
				}

				kafkaRecords := make(map[string]interface{})
				kafkaRecords["records"] = logsToSend

				if math.Mod(float64(sizeCheckCounter), 10.0) == 0 {
					kafkaData, err := json.Marshal(kafkaRecords)
					if err != nil {
						log.Print(err)
						os.Exit(1)
					}

					size := len(kafkaData)

					if size >= int(config.MaxBulkSize) {
						return true
					}
				}

				return false
			}() {

				copyOfLogsToSend := make([]map[string]interface{}, len(logsToSend))
				copy(copyOfLogsToSend, logsToSend)
				logsToSend = make([]map[string]interface{}, 0)

				go sendToKafka(copyOfLogsToSend, config, topicName)

				if totalLogsToSend <= 0 {
					fmt.Printf("Completed sending %d logs to kafka in %f minutes\n", int(config.LogsPerMin)-totalLogsToSend, time.Since(startTime).Minutes())
					return
				} else if logsToSendInThisFlush <= 0 {
					break
				}
			}
		}
	}
}

func processLogTemlates(args []string) []string {

	var allLogs []string

	noOfLogTemplates := len(args) - 2

	for i := 0; i < noOfLogTemplates; i++ {

		file, err := os.Open(args[i+2])

		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		defer file.Close()

		reader := bufio.NewReader(file)

		for {

			line, err := reader.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatal(err)
			}

			allLogs = append(allLogs, strings.TrimSpace(line))
		}
	}

	return allLogs
}

func loadConfig(path string) (*Config, error) {

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func main() {

	args := os.Args
	if len(args) < 3 {
		fmt.Println("Insufficient arguments")
		return
	}

	config, err := loadConfig(args[1])
	if err != nil {
		fmt.Println(err)
		return
	}

	if len(config.KafkaTopics) <= 0 || config.FlushInterval > 60 || config.MaxBulkCount > config.LogsPerMin {
		fmt.Println("Either 'kafka_topics is empty' or 'flush_interval is greater than 60' or 'max_bulk_count is greater than logs_per_min'")
		return
	}

	allLogs := processLogTemlates(args)

	for range time.Tick(time.Minute) {
		for _, topicName := range config.KafkaTopics {
			fmt.Printf("Starting to send 1 minute logs to topic %s\n", topicName)
			go generateLogsForOneMinute(time.Now(), config, allLogs, topicName)
		}
	}
}
