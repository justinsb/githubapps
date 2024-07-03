package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	time.Sleep(5 * time.Minute)
	return nil
}
