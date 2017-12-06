package main

import (
	"os"
	"fmt"
	"encoding/json"
	"time"
	"strings"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/apex/go-apex"
)

var (
	dynamoTableName = os.Getenv("REPORTER_DYNAMO_TABLE")
)

type Server struct {
	HostPort         string `json:"server"`
	SSL              bool   `json:"secure"`
	SkipVerification bool   `json:"skip-verification"`
}

type Report struct {
	Server Server
	Error error
}

func parseServerString(serverString string) []Server {
	s := make([]Server, 0)

	for _, rawServer := range strings.Split(serverString, ";") {
		server := Server{
			HostPort: "",
			SSL: false,
			SkipVerification: false,
		}

		hp := strings.Split(rawServer, "/")
		host := hp[0]
		port := hp[1]

		if strings.HasPrefix(port, "+") {
			server.SSL = true
			port = strings.TrimPrefix(port, "+")

			if strings.HasPrefix(port, "?") {
				server.SkipVerification = true
				port = strings.TrimPrefix(port, "?")
			}
		}

		server.HostPort = strings.Join([]string{host, port}, ":")

		s = append(s, server)
	}

	return s
}

func sendAlert(report Report) {
	var message string

	form := url.Values{}
	form.Add("token", os.Getenv("PUSHOVER_TOKEN"))
	form.Add("user", os.Getenv("PUSHOVER_USERS"))

	if report.Error != nil {
		message = fmt.Sprintf("Server %s is not responding to incoming connections. SSL: %t, verification skipped: %t", report.Server.HostPort, report.Server.SSL, report.Server.SkipVerification)
		form.Add("priority", "2")
		form.Add("retry", "120")
		form.Add("expire", "3600")
	} else {
		message = fmt.Sprintf("Server %s has come back online.", report.Server.HostPort)
		form.Add("priority", "0")
	}

	form.Add("message", message)

	_, _ = http.PostForm("https://api.pushover.net/1/messages.json", form)
}

func getIsDown(dynamoInstance *dynamodb.DynamoDB, server Server) (bool, error) {
	getKeyValue := &dynamodb.AttributeValue{}
	getKeyValue.SetS(server.HostPort)

	getKeyMap := make(map[string]*dynamodb.AttributeValue)
	getKeyMap["server"] = getKeyValue

	getInput := &dynamodb.GetItemInput{
		TableName: &dynamoTableName,
		Key: getKeyMap,
	}

	getOutput, err := dynamoInstance.GetItem(getInput)
	if err != nil {
		return false, err
	}

	return *(getOutput.Item["isCurrentlyDown"].BOOL), nil
}

func setIsDown(dynamoInstance *dynamodb.DynamoDB, report Report) error {
	tableKey := &dynamodb.AttributeValue{}
	tableKey.SetS(report.Server.HostPort)
	tableKeyMap := make(map[string]*dynamodb.AttributeValue)
	tableKeyMap["server"] = tableKey
	
	isDownValue := &dynamodb.AttributeValue{}
	isDownValue.SetBOOL(report.Error != nil)
	
	attributeUpdateEntry := &dynamodb.AttributeValueUpdate{}
	attributeUpdateEntry.SetAction("PUT")
	attributeUpdateEntry.SetValue(isDownValue)

	attributeUpdateMap := make(map[string]*dynamodb.AttributeValueUpdate)
	attributeUpdateMap["isCurrentlyDown"] = attributeUpdateEntry

	setInput := &dynamodb.UpdateItemInput{
		Key: tableKeyMap,
		AttributeUpdates: attributeUpdateMap,
		TableName: &dynamoTableName,
	}
	
	_, err := dynamoInstance.UpdateItem(setInput)
	if err != nil {
		return err
	}

	return nil
}

func getHealthcheckLambdaName() string {
	healthcheck := "healthcheck"
	current := os.Getenv("APEX_FUNCTION_NAME")
	current_full := os.Getenv("LAMBDA_FUNCTION_NAME")
	prefix := strings.TrimSuffix(current_full, current)

	return prefix + healthcheck
}

func reportForServer(server Server, lambdaInstance *lambda.Lambda, reportChannel chan Report) {
	serverJson, err := json.Marshal(server)
	if err != nil {
		reportChannel <- Report{Server: server, Error: err}
		return
	}

	functionName := getHealthcheckLambdaName()
	invokeInput := &lambda.InvokeInput{
		FunctionName: &functionName,
		Payload: serverJson,
	}

	invokeOutput, err := lambdaInstance.Invoke(invokeInput)
	if err != nil {
		reportChannel <- Report{Server: server, Error: err}
		return
	}

	output := ""
	err = json.Unmarshal(invokeOutput.Payload, &output)
	if err != nil {
		reportChannel <- Report{Server: server, Error: err}
		return
	}

	reportChannel <- Report{Server: server, Error: nil}
	return
}

func runJob(event json.RawMessage, ctx *apex.Context) (interface{}, error) {
	started := time.Now()

	sess := session.Must(session.NewSession())
	lambdaInstance := lambda.New(sess)
	dynamoInstance := dynamodb.New(sess)

	servers := parseServerString(os.Getenv("REPORTER_SERVERS"))

	reportChannel := make(chan Report)

	for _, server := range servers {
		go reportForServer(server, lambdaInstance, reportChannel)
	}

	reports := make([]Report, 0)
	failures := 0

	for {
		select {
		case <-time.After(10 * time.Millisecond):
			if len(reports) == len(servers) {
				out := make(map[string]interface{})
				out["total"] = len(servers)
				out["failures"] = failures

				return out, nil
			}

			if time.Since(started) > 1 * time.Minute {
				return nil, fmt.Errorf("Timeout reached")
			}

			continue

		case report := <-reportChannel:
			reports = append(reports, report)

			isDown, getErr := getIsDown(dynamoInstance, report.Server)

			if report.Error != nil {
				failures = failures + 1

				if getErr == nil {
					if !isDown {
						sendAlert(report)
					}
				}
			} else {
				if isDown {
					sendAlert(report)
				}
			}
				

			_ = setIsDown(dynamoInstance, report)
		}
	}


	return nil, fmt.Errorf("Loop escaped, for some reason")
}

func main() {
	apex.HandleFunc(runJob)
}
