package main

import (
	"fmt"
	"io"
	"net/http"
)

func main() {
	// Note: 'opengrok' matches the service name in your docker-compose.yml
	url := "http://opengrok:8080/api/v1/search?full=Jenkins"

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Connection Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, string(body))
}
