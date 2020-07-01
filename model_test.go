package accessmodel

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"
)

func TestModel1(t *testing.T) {
	accessModel := NewAccessModel("model.xml", "save.xml", 100000)
	stat := accessModel.NewStatistic(context.Background())
	curr := time.Now().Unix()
	currCount := 0

	for {
		err := accessModel.Request()
		if err != nil {
			fmt.Println(err)
		} else {
			now := time.Now().Unix()
			if curr != now {
				log.Println(curr, currCount)

				curr = now
				currCount = 1
			} else {
				currCount++
			}

			stat.Add(1)
		}
	}
}
