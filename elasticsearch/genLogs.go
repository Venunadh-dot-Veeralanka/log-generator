package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/natefinch/lumberjack"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tags map[string]string

type SnappyFlowKeyData struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Type      string `json:"type"`
	ProfileID string `json:"profile_id"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Protocol  string `json:"protocol"`
}

type Config struct {
	FilePath       string  `json:"file_path"`
	FileName       string  `json:"file_name"`
	FileSizeRotate uint64  `json:"file_size_rotate"`
	Compress       bool    `json:"compress_old_logs"`
	BulkSize       float64 `json:"bulk_size"`
	ESKey          string  `json:"es_key"`
	ProjectName    string  `json:"project_name"`
	Tags           Tags    `json:"tags"`
	LogsPerMin     uint64  `json:"logs_per_min"`
	LogInterval    float64 `json:"log_interval"`
	TimeFormat     string  `json:"time_format"`
	FileWrite      bool    `json:"file_write"`
	ESSend         bool    `json:"es_send"`
}

func processLogTemlates(args []string) [][][]string {
	var logTemplates [][][]string
	noOfLogTemplates := len(args) - 2
	for i := 0; i < noOfLogTemplates; i++ {
		file, err := os.Open(args[i+2])
		defer file.Close()

		if err != nil {
			log.Fatal(err)
		}

		reader := bufio.NewReader(file)

		var logTemplate [][]string
		for {
			line, err := reader.ReadString('\n')

			if err == io.EOF {
				break
			}

			if err != nil {
				log.Fatal(err)
			}

			var logInfo []string
			line = strings.TrimSpace(line)
			index := strings.Index(line, ",")

			if index == -1 {
				continue
			}

			index1 := strings.Index(line, "=")
			if index1 == -1 {
				continue
			}

			logInfo = append(logInfo, strings.TrimSpace(line[index1+1:index]))

			msgInfo := line[index+1:]
			index1 = strings.Index(msgInfo, "=")
			if index1 == -1 {
				continue
			}

			logInfo = append(logInfo, strings.TrimSpace(msgInfo[index1+1:]))
			logTemplate = append(logTemplate, logInfo)
		}
		if len(logTemplate) > 0 {
			logTemplates = append(logTemplates, logTemplate)
		}
	}
	return logTemplates
}

func startLogGeneration(config *Config, esConfig SnappyFlowKeyData, logTemplates [][][]string) {
	var logsPerInterval uint64 = uint64(math.Ceil(float64(config.LogsPerMin) / 60.0 * config.LogInterval))
	//fmt.Println(logsPerInterval)
	for {
		fmt.Println("---------------------------------------------------------------------")
		start := time.Now()
		fmt.Println(start)
		fmt.Println()

		noOfLogtemplates := len(logTemplates)
		routineDone := make(chan bool, noOfLogtemplates)
		mutex := &sync.Mutex{}

		var logsPerRoutine uint64 = logsPerInterval / uint64(len(logTemplates))
		for i := 0; i < len(logTemplates); i++ {
			go createLog(config, esConfig, logTemplates[i], len(logTemplates[i]), config.TimeFormat, logsPerRoutine, mutex, routineDone)
		}

		for i := 0; i < noOfLogtemplates; i++ {
			<-routineDone
		}
		close(routineDone)

		fmt.Println()
		fmt.Println(time.Now())
		elapsedTime := time.Since(start).Seconds()
		fmt.Printf("Log Generation took %f\n", elapsedTime)

		sleepTime := config.LogInterval - elapsedTime
		if sleepTime > 0 {
			fmt.Printf("Sleep for %f\n", sleepTime)
		}

		fmt.Println("======================================================================")
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}

func createLog(config *Config, esConfig SnappyFlowKeyData, logTemplate [][]string, size int, timeFormat string, logsPerRoutine uint64, mutex *sync.Mutex, routineDone chan<- bool) {

	var esLogs [][]byte
	//var logLines string = ""
	var logLine string = ""

	seed := rand.NewSource(time.Now().UnixNano())
	r := rand.New(seed)
	var count uint64 = 0
	var esLogsSize uint64 = 0
	for {
		time, level, msg := generateLogLevelMsg(config.Tags, timeFormat, r, logTemplate[r.Intn(size)])
		if config.FileWrite {
			logLine = time + " " + level + " " + msg + "\n"
			//logLines = logLines + logLine
		}
		if config.ESSend {
			esLogs = generateESLog(config.Tags, level, msg, esLogs, &esLogsSize)
		}

		count++
		//Write 100 logs in buffer to file
		//if count == logsPerRoutine || count%100 == 0 {
		//fmt.Printf("es logs size: %d\n",esLogsSize)
		if config.FileWrite {
			mutex.Lock()
			err := writeLogsToFile(config, []byte(logLine))
			if err != nil {
				fmt.Println(err)
			}
			mutex.Unlock()
			logLine = ""
		}
		if count == logsPerRoutine || ((float64)(esLogsSize/1024)/1024) >= config.BulkSize {
			if config.ESSend {
				sendToElasticSearch(config, esConfig, esLogs)
				esLogs = esLogs[:0]
				esLogsSize = 0
			}
			if count == logsPerRoutine {
				break
			}
		}
	}
	routineDone <- true
}

func writeLogsToFile(config *Config, logs []byte) error {
	filePath := config.FilePath + "/" + config.FileName
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	/*
	   	stat, err := f.Stat()
	     	if err != nil {
	       	return err
	     	}
	   	//fmt.Println(stat)
	     	var bytes int64
	     	bytes = stat.Size()

	   	//var kilobytes int64
	   	//kilobytes = (bytes / 1024)

	   	var megabytes float64
	   	megabytes = (float64)(bytes/1024) / 1024
	   	//fmt.Println("File size in megabytes ", megabytes)

	   	if megabytes >= (float64)(config.FileSize) {
	   		srcName := stat.Name()
	   		lastChar := srcName[len(srcName)-1:]
	   		fmt.Println(lastChar)
	   		i, err := strconv.Atoi(lastChar)
	   		newName := ""
	   		if err != nil {
	   			newName = srcName + ".1"
	   			fmt.Println(err)
	   		} else {
	   			newName = srcName[:len(srcName)-1] + strconv.Itoa(i+1)
	   			fmt.Println(newName)
	   		}
	   		os.Rename(srcName, newName)
	   		f, err = os.OpenFile(filePath, os.O_CREATE, 0666)
	       	if err != nil {
	           	return err
	   		}
	       }
	*/
	_, err = f.Write(logs)
	if err != nil {
		return err
	}

	err = f.Sync()
	if err != nil {
		return err
	}

	err = rotateLogFile(filePath, config)
	if err != nil {
		return err
	}

	return nil
	//fmt.Println("Logs written Successfully.")
}

func generateLogLevelMsg(tagMap map[string]string, timeFormat string, r *rand.Rand, logInfo []string) (string, string, string) {
	msg := logInfo[1]
	for {
		index := strings.Index(msg, "$IP")
		if index == -1 {
			break
		}

		ip := strconv.Itoa(r.Intn(256)) + "." + strconv.Itoa(r.Intn(256)) + "." + strconv.Itoa(r.Intn(256)) + "." + strconv.Itoa(r.Intn(256))
		msg = msg[:index] + ip + msg[index+3:]
	}
	for {
		index := strings.Index(msg, "$INT")
		if index == -1 {
			break
		}
		port := strconv.Itoa(r.Intn(65535) + 1)

		msg = msg[:index] + port + msg[index+4:]
	}
	level := strings.ToUpper(logInfo[0])
	time := time.Now().Format(timeFormat)

	return time, level, msg
}

func generateESLog(tagMap map[string]string, level string, msg string, logs [][]byte, esLogsSize *uint64) [][]byte {

	data := make(map[string]interface{})
	for k, v := range tagMap {
		data[k] = v
	}
	data["level"] = level
	data["message"] = msg
	data["time"] = time.Now().Unix() * 1000

	jsonStr, err := json.Marshal(data)
	if err != nil {
		log.Print(err)
		return logs
	}

	logs = append(logs, jsonStr)
	//fmt.Println(string(jsonStr))
	*esLogsSize += uint64(len(jsonStr))

	return logs
}

func sendToElasticSearch(config *Config, esConfig SnappyFlowKeyData, logs [][]byte) {

	index := strings.ToLower(fmt.Sprintf("log-%s-%s", esConfig.ProfileID, GetAPMName(config.ProjectName)))
	esPath := fmt.Sprintf("%s://%s:%s/%s", esConfig.Protocol, esConfig.Host, strconv.Itoa(esConfig.Port), index)

	esUrl := esPath + "_write/doc/_bulk"
	//fmt.Println(index)
	//fmt.Println(esUrl)
	noOfLogs := len(logs)
	if noOfLogs == 0 {
		fmt.Println("No Logs")
		return
	}

	fmt.Println("Current time is %s", time.Now())
	fmt.Println("logs being sent are", logs)

	var esData []byte
	for _, elem := range logs {
		esData = append(esData, "{\"index\":{}}\n"...)
		esData = append(esData, elem...)
		esData = append(esData, "\n"...)
	}
	esData = append(esData, "\n"...)

	//fmt.Printf("No of records to be sent %d\n", len(logs))
	req, err := http.NewRequest("POST", esUrl, bytes.NewBuffer(esData))
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(esConfig.Username, esConfig.Password)

	client := &http.Client{Timeout: 30 * time.Second}

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		fmt.Printf("Successfully sent %d records to ES: %s \n", noOfLogs, esUrl)
	} else {
		fmt.Println("Failed to send ES documents", res.Status)
	}
}

func LoadConfig(path string) (*Config, error) {
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

func setTimeFormats(timesFormats map[string]bool) {

	timesFormats["Sun Jan 2 15:04:05 2006"] = true
	timesFormats["2006-01-02-15.04.05.000000"] = true
	timesFormats["2006-01-02 15:04:05,000"] = true
	timesFormats["20060102-15:04:05:000"] = true
	timesFormats["Jan 2 15:04:05"] = true
	timesFormats["2006-02-01_15:04:05"] = true
	timesFormats["01.02 15:04:05"] = true
	timesFormats["06/01/02 15:04:05"] = true
	timesFormats["2006/01/02 15:04:05"] = true
	timesFormats["2006-01-02 15:04:05"] = true
	timesFormats["2006-01-02 15:04:05,000"] = true
	timesFormats["2006-01-02'T'15:04:05*000+0000"] = true
	timesFormats["2006-01-02T15:04:05+0000"] = true
	timesFormats["2006-01-02T15:04:05.000'+0000'"] = true

}

func checkConfigValidity(config *Config, timesFormats map[string]bool) error {
	if timesFormats[config.TimeFormat] == false {
		for k, _ := range timesFormats {
			fmt.Println(k)
		}

		return errors.New("Invalid time format. Please select from above valid formats")
	}

	if config.ESSend && config.ESKey == "" {
		return errors.New("Elastic Search key is not provided")
	}

	return nil
}

func rotateLogFile(filePath string, config *Config) error {
	var f, err = os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	defer f.Close()

	var bytes int64
	bytes = stat.Size()
	var megabytes float64
	megabytes = (float64)(bytes/1024) / 1024
	//fmt.Printf("File size in megabytes %f\n", megabytes)

	if megabytes > 0.0 && megabytes >= (float64)(config.FileSizeRotate) {
		fmt.Printf("File size in MB: %f, config file size:%d \n", megabytes, config.FileSizeRotate)
		log := log.New(f, "", log.Ldate|log.Ltime)

		l := &lumberjack.Logger{
			Filename: filePath,
			Compress: config.Compress,
		}

		log.SetOutput(l)

		l.Rotate()
	}
	return nil
}

func main() {
	args := os.Args

	if len(args) < 3 {
		fmt.Println("Insufficient arguments")
		return
	}

	config, err := LoadConfig(args[1])
	if err != nil {
		fmt.Println(err)
		return
	}

	var timeFormats = make(map[string]bool)
	setTimeFormats(timeFormats)

	if err := checkConfigValidity(config, timeFormats); err != nil {
		fmt.Println(err)
		return
	}

	var esConfig SnappyFlowKeyData
	logTemplates := processLogTemlates(args)
	if config.ESSend == true {
		esConfig, err = createTargetsFromKey(config)
		fmt.Println(esConfig)
		if err != nil {
			fmt.Println("Decryption Failed")
			return
		}
	}

	startLogGeneration(config, esConfig, logTemplates)

}
