package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

// JSONError represents a JSON Error
type JSONError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// JSONProgress represents a JSON-encoded progress instance
type JSONProgress struct {
	//terminalFd uintptr
	Current int64 `json:"current,omitempty"`
	Total   int64 `json:"total,omitempty"`
	Start   int64 `json:"start,omitempty"`
}

// JSONMessage represents a JSON-encoded message regarding the status of a stream
type JSONMessage struct {
	Stream          string        `json:"stream,omitempty"`
	Status          string        `json:"status,omitempty"`
	Progress        *JSONProgress `json:"progressDetail,omitempty"`
	ProgressMessage string        `json:"progress,omitempty"` //deprecated
	ID              string        `json:"id,omitempty"`
	From            string        `json:"from,omitempty"`
	Time            int64         `json:"time,omitempty"`
	TimeNano        int64         `json:"timeNano,omitempty"`
	Error           *JSONError    `json:"errorDetail,omitempty"`
	ErrorMessage    string        `json:"error,omitempty"` //deprecated
	// Aux contains out-of-band data, such as digests for push signing.
	Aux *json.RawMessage `json:"aux,omitempty"`
}

func PullImage(ctx context.Context, dClient client.CommonAPIClient, imageToPull string) error {
	// Pull all missing images.
	log.Info("Pulling required images... (this may take a while)")
	if err := pullImage(ctx, dClient, types.ImagePullOptions{}, imageToPull); err != nil {
		return fmt.Errorf("unable to pull image %s: %s", imageToPull, err)
	}

	log.Info("Completed pulling required images")

	return nil
}

func pullImage(ctx context.Context, dClient client.CommonAPIClient, pullOptions types.ImagePullOptions, imageRef string) error {
	log.Infof("Pulling image: %s", imageRef)
	progress, err := dClient.ImagePull(ctx, imageRef, pullOptions)
	if err != nil {
		return err // Not wrapped because there isn't any context to add here.
	}
	defer progress.Close()

	scanner := bufio.NewScanner(progress)
	for scanner.Scan() {
		line := scanner.Text()
		var msg JSONMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Debugf("Malformed progress line: %s - %s", line, err)
		} else {
			if msg.Error != nil {
				return fmt.Errorf("unable to pull image %s: %s", imageRef, msg.Error.Message)
			}
		}
		if msg.Progress != nil && msg.Progress.Total > 0 {
			log.Debugf("    %s %s layer %s %0.2f%%", imageRef, msg.ID, msg.Status, float64(msg.Progress.Current)/float64(msg.Progress.Total)*100)
		} else {
			log.Debugf("    %s %s %s", imageRef, msg.ID, msg.Status)
		}
	}

	return nil
}
