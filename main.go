package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/golang-jwt/jwt/v4"
)

type Request struct {
	TenanName string `json:"tenanName"`
}

type DBdata struct {
	AuthStatus bool     `json:"authStatus"`
	Email      string   `json:"email"`
	IsProduct  []string `json:"isProduct"`
	Tenan      string   `json:"tenan"`
	Type       string   `json:"type"`
}

type Claims struct {
	Data DBdata `json:"data"`
	jwt.RegisteredClaims
}

type Payload struct {
	TenanName string `json:"tenanName"`
	BusName   string `json:"busName"`
}

func getFileFromS3(bucket, key string, region string) (string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("unable to load SDK config, %v", err)
	}

	client := s3.NewFromConfig(cfg)

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := client.GetObject(context.TODO(), getObjectInput)
	if err != nil {
		return "", fmt.Errorf("failed to get file from S3, %v", err)
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read file content, %v", err)
	}

	return string(body), nil
}

func ValidateToken(tokens string) (int, string, string, error) {
	// fmt.Println("in ValidateToken")
	var REGION = "ap-southeast-1"
	var BUCKET = "cdk-hnb659fds-assets-058264531773-ap-southeast-1"
	var KEYFILE = "token.txt"
	setKey, err := getFileFromS3(BUCKET, KEYFILE, REGION)
	jwtKey := []byte(setKey)
	if err != nil {
		return 500, "Internal server error", "Internal server error", err
	}
	tokenString := strings.TrimPrefix(tokens, "Bearer ")
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtKey, nil
	})

	if err != nil {
		// fmt.Println("err ====> ", err)
		if err == jwt.ErrSignatureInvalid {
			return 401, "unauthorized", "unauthorized", err
		}
		return 401, "unauthorized", "unauthorized", err
	}

	if !token.Valid {
		return 401, "unauthorized", "unauthorized", err
	}

	return 200, claims.Data.Tenan, claims.Data.Type, nil
}

func CheckTable(tenan string) (bool, string, error) {
	// fmt.Println("tenan => ", tenan)
	var isTeana string
	var genTenanName string = tenan + "_" + "demo_customer"
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := dynamodb.New(sess)
	input := &dynamodb.ListTablesInput{}
	result, err := svc.ListTables(input)
	if err != nil {
		fmt.Println("Error listing tables:", err)
		return false, "", err
	}

	// Print the table names
	fmt.Println("Tables:")
	for _, tableName := range result.TableNames {
		// fmt.Println("tableName => ", *tableName)
		strPointerValue := *tableName
		if strPointerValue == genTenanName {
			isTeana = genTenanName
		}
	}

	if isTeana == genTenanName {
		return false, "", nil
	}

	return true, genTenanName, nil

}

func EventBusSend(ctx context.Context, tenanName string) (bool, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	svc := eventbridge.NewFromConfig(cfg)

	payload := Payload{
		TenanName: tenanName,
		BusName:   "bus-superadmin-create-tenan",
	}

	detail, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	setInput := &eventbridge.PutEventsInput{
		Entries: []types.PutEventsRequestEntry{
			{
				Time:         aws.Time(time.Now()),
				Detail:       aws.String(string(detail)),
				DetailType:   aws.String("Message"),
				EventBusName: aws.String("arn:aws:events:ap-southeast-1:058264531773:event-bus/bus-superadmin-create-tenan"),
				Source:       aws.String("lambda publish"),
				// Resources:    setResources,
			},
		},
	}

	_, err = svc.PutEvents(ctx, setInput)
	if err != nil {
		return false, err
	}
	return true, nil
}

// func CreateTable(tableName string) (bool, error) {
// 	fmt.Println("start create table.")
// 	sess := session.Must(session.NewSessionWithOptions(session.Options{
// 		SharedConfigState: session.SharedConfigEnable,
// 	}))

// 	svc := dynamodb.New(sess)

// 	input := &dynamodb.CreateTableInput{
// 		TableName: aws.String(tableName),
// 		AttributeDefinitions: []*dynamodb.AttributeDefinition{
// 			{
// 				AttributeName: aws.String("customerID"),
// 				AttributeType: aws.String("S"), // S for String
// 			},
// 		},
// 		KeySchema: []*dynamodb.KeySchemaElement{
// 			{
// 				AttributeName: aws.String("customerID"),
// 				KeyType:       aws.String("HASH"), // Partition key
// 			},
// 		},
// 		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
// 			ReadCapacityUnits:  aws.Int64(5),
// 			WriteCapacityUnits: aws.Int64(5),
// 		},
// 	}

// 	_, err := svc.CreateTable(input)
// 	if err != nil {
// 		fmt.Println("Error creating table:", err)
// 		return false, err
// 	}

// 	fmt.Println("Table", tableName, "created successfully!")

// 	return true, nil
// }

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// fmt.Println("req", req)
	// if req.HTTPMethod != http.MethodPost {
	// 	return events.APIGatewayProxyResponse{
	// 		StatusCode: http.StatusMethodNotAllowed,
	// 		Body:       "Method Not Allowed",
	// 	}, nil
	// }
	var token = req.Headers["authorization"]
	var data Request
	err := json.Unmarshal([]byte(req.Body), &data)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       fmt.Sprintf("Error parsing request body: %v", err),
		}, err
	}

	status, _, userType, err := ValidateToken(token)
	if err != nil {
		fmt.Println("err validate token => ", err)
		return events.APIGatewayProxyResponse{StatusCode: status, Body: userType}, nil
	}
	if status != 200 {
		fmt.Println("status error token => ", err)
		return events.APIGatewayProxyResponse{StatusCode: status, Body: userType}, nil
	}
	if userType != "super_admin" {
		return events.APIGatewayProxyResponse{
			StatusCode: 401,
			Body:       "unauthorized",
		}, nil
	}

	tableStatus, tableName, err := CheckTable(data.TenanName)
	if err != nil {
		fmt.Println(err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}
	if tableStatus == false {
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       "this tenan alreadly exists.",
		}, nil
	}

	busStatus, err := EventBusSend(ctx, tableName)

	// createTableStatus, err := CreateTable(tableName)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Send bus fail",
		}, nil
	}
	if busStatus != true {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Send bus fail",
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}

func main() {
	lambda.Start(handler)
}
