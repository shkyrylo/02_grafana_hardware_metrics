package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "go.mongodb.org/mongo-driver/bson"
)

type User struct {
	ID    interface{} `json:"id" bson:"_id,omitempty"`
	Name  string      `json:"name" bson:"name"`
	Email string      `json:"email" bson:"email"`
}

var (
	mongoConn   *mongo.Database
	elasticConn *elasticsearch.Client
)

func main() {
	var err error

	mongoConn, err = newMongoConn()
	if err != nil {
		log.Printf("Failed to connect to MongoDB: %v", err)
	}

	elasticConn, err = newElasticConn()
	if err != nil {
		log.Printf("Failed to connect to Elasticsearch: %v", err)
	}

	http.HandleFunc("/users", handleUsers)
	log.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func newElasticConn() (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://elasticsearch:9200",
		},
	}
	client, err := elasticsearch.NewClient(cfg)

	_, err = client.Ping()
	if err != nil {
		log.Printf("Error pinging Elasticsearch: %v", err)
	}

	return client, err
}

func newMongoConn() (*mongo.Database, error) {
	mongoURI := fmt.Sprintf("mongodb://%s:%s@mongo:27017",
		os.Getenv("MONGO_USER"),
		os.Getenv("MONGO_PASSWORD"),
	)

	clientOptions := options.Client().ApplyURI(mongoURI)
	mongoClient, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Printf("Failed to connect to MongoDB: %v", err)
	}

	err = mongoClient.Ping(context.Background(), nil)
	if err != nil {
		log.Printf("MongoDB ping failed: %v", err)
	}

	mongoConn = mongoClient.Database(os.Getenv("MONGO_DB"))

	return mongoConn, err
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var user User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			log.Printf("Failed to decode user: %v", err)
			return
		}

		collection := mongoConn.Collection("users")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := collection.InsertOne(ctx, user)
		if err != nil {
			http.Error(w, "Failed to save to MongoDB", http.StatusInternalServerError)
			return
		}
		user.ID = result.InsertedID

		data, err := json.Marshal(user)
		if err != nil {
			http.Error(w, "Failed to process data", http.StatusInternalServerError)
			return
		}
		log.Printf("Marshalled user data: %s", string(data))

		res, err := elasticConn.Index("users", bytes.NewReader(data))
		if err != nil {
			http.Error(w, "Failed to save to Elasticsearch", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()

		if res.IsError() {
			http.Error(w, "Failed to index user", http.StatusInternalServerError)
			return
		}

		log.Printf("User indexed successfully in Elasticsearch: %s", string(data))

		delay := generateDelay()
		time.Sleep(delay) // Simulate delay

		fmt.Printf("Delay: %v\n", delay)

		w.WriteHeader(http.StatusCreated)
		w.Write(data)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func generateDelay() time.Duration {
	delay := time.Duration(rand.Intn(900)+100) * time.Millisecond

	return delay
}
