package accessmodel

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"
)

var (
	ErrorNothingToSave = fmt.Errorf("nothing to save")
)

type InnerStatistic struct {
	Count []int `xml:"Count"`

	outer   *Statistic
	date    string
	saveXml string
}

type Statistic struct {
	mutex   sync.Mutex
	inner   *InnerStatistic
	saveXml string
	once    sync.Once
}

func NewStatistic(saveXml string) *Statistic {
	return &Statistic{
		saveXml: saveXml,
	}
}

func (s *Statistic) AutoSave(ctx context.Context) {
	s.once.Do(func() {
		go s.autoSave(ctx)
	})
}

func (s *Statistic) autoSave(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recover error", err)
		}
	}()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			err := s.Save()
			if err != nil {
				fmt.Println("autoSave done error", err)
			} else {
				fmt.Println("autoSave done")
			}
			return
		case <-ticker.C:
			err := s.Save()
			if err != nil {
				fmt.Println("autoSave ticker error", err)
			}
		}
	}
}

func (s *Statistic) prepareSaveData() ([]byte, string, error) {
	if s.inner == nil {
		return nil, "", ErrorNothingToSave
	}

	data, err := xml.MarshalIndent(s.inner, "", "    ")
	if err != nil {
		return nil, "", err
	}

	return data, s.inner.saveXml, nil
}

func (s *Statistic) Add(delta int) error {
	now := time.Now()
	dateString := now.Format("2006-01-02")

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.inner != nil && s.inner.date != dateString {
		data, saveXml, err := s.prepareSaveData()
		if err == ErrorNothingToSave {
		} else if err != nil {
			return err
		} else {
			err = ioutil.WriteFile(saveXml, data, 0777)
			if err != nil {
				return err
			}
		}

		s.inner = nil
	}

	if s.inner == nil {
		inner, err := s.loadInnerStatistic(dateString)
		if err != nil {
			return err
		}

		s.inner = inner
	}

	return s.inner.add(delta, now)
}

func (s *Statistic) loadInnerStatistic(dateString string) (*InnerStatistic, error) {
	saveXml := fmt.Sprintf("%s.statistic.%s.xml", s.saveXml, dateString)

	exists, err := s.pathExists(saveXml)
	if err != nil {
		return nil, err
	}

	if exists {
		data, err := ioutil.ReadFile(saveXml)
		if err != nil {
			return nil, err
		}

		inner := InnerStatistic{}
		err = xml.Unmarshal(data, &inner)
		if err != nil {
			return nil, err
		}

		inner.saveXml = saveXml
		inner.outer = s
		inner.date = dateString

		return &inner, nil
	} else {
		inner := InnerStatistic{
			Count:   make([]int, 48),
			outer:   s,
			date:    dateString,
			saveXml: saveXml,
		}

		return &inner, nil
	}
}

func (s *Statistic) Save() error {
	s.mutex.Lock()
	data, saveXml, err := s.prepareSaveData()
	s.mutex.Unlock()
	if err != nil {
		if err == ErrorNothingToSave {
			return nil
		}
		return err
	}

	return ioutil.WriteFile(saveXml, data, 0777)
}

func (s *Statistic) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	} else {
		return true, nil
	}
}

func (is *InnerStatistic) add(delta int, now time.Time) error {
	hour, minute, _ := now.Clock()
	k := hour*2 + minute/30

	is.Count[k%48] += delta

	return nil
}
