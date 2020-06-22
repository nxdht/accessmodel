package accessmodel

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	errorRetry = fmt.Errorf("retry")
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type AccessModel struct {
	modelXml         string
	saveXml          string
	totalCountPerDay int

	mutex sync.RWMutex
	inner *InnerAccessModel
}

type Model struct {
	Access []float64 `xml:"Access"`
}

type AModel struct {
	Model            Model   `xml:"Model"`
	DeviationPerDay  float64 `xml:"DeviationPerDay"`
	DeviationPerHour float64 `xml:"DeviationPerHour"`
}

func (am *AModel) sumAccesses() float64 {
	var f float64
	for _, a := range am.Model.Access {
		f += a
	}
	return f
}

type InnerAccessModel struct {
	AModel         *AModel `xml:"AModel"`
	RunModel       string  `xml:"RunModel"`
	TotalCount     int     `xml:"TotalCount"`
	TotalCountReal int     `xml:"TotalCountReal"`
	GroupsPerHour  int     `xml:"GroupsPerHour"`
	Unit           int     `xml:"Unit"`

	outer             *AccessModel
	mutex             sync.Mutex
	date              string
	saveXml           string
	models            []int
	currentModelIndex int
	currentData       []int
}

func (am *InnerAccessModel) random() error {
	groupsPerHour := 3600

	r := rand.Float64()*am.AModel.DeviationPerDay*2 - am.AModel.DeviationPerDay
	tc := float64(am.outer.totalCountPerDay) * (1 + r)
	sa := am.AModel.sumAccesses()

	var tcr int
	var runModel []string
	var models []int
	for _, a := range am.AModel.Model.Access {
		r = rand.Float64()*am.AModel.DeviationPerHour*2 - am.AModel.DeviationPerHour
		c := (tc * a / sa) * (1 + r)

		group := make([]int, groupsPerHour)
		for i := 0; i < int(c); i++ {
			r := rand.Int31n(int32(groupsPerHour))
			group[r]++
		}

		for _, g := range group {
			runModel = append(runModel, fmt.Sprintf("%d", uint(g)))
			models = append(models, int(g))

			tcr += int(g)
		}
	}

	runModelString := strings.Join(runModel, ",")

	am.GroupsPerHour = groupsPerHour
	am.TotalCount = am.outer.totalCountPerDay
	am.TotalCountReal = tcr
	am.Unit = int(time.Hour.Milliseconds() / int64(groupsPerHour))
	am.RunModel = runModelString

	am.models = models

	return nil
}

func (am *InnerAccessModel) calcModels() error {
	var models []int

	s := strings.Split(am.RunModel, ",")
	for _, r := range s {
		k, err := strconv.Atoi(r)
		if err != nil {
			return err
		}
		models = append(models, k)
	}

	am.models = models

	return nil
}

func (am *InnerAccessModel) request(now time.Time) (err error) {
	hour, minute, second := now.Clock()
	k := hour*3600 + minute*60 + second

	am.mutex.Lock()

	if am.currentData == nil || am.currentModelIndex != k {
		m := am.models[k]

		am.currentData = make([]int, 10)
		for i := 0; i < m; i++ {
			r := rand.Intn(10)
			am.currentData[r]++
		}

		am.currentModelIndex = k
	}

	first := -1
	ms := now.Nanosecond() / 1000000
	idx := ms / 100
	if am.currentData[idx] > 0 {
		am.currentData[idx]--
	} else {
		for i := idx + 1; i < len(am.currentData); i++ {
			if am.currentData[i] > 0 {
				first = i
				break
			}
		}

		err = errorRetry
	}

	am.mutex.Unlock()

	if err != nil {
		if first == -1 {
			<-time.NewTimer(time.Millisecond * time.Duration(1000-ms)).C
		} else {
			<-time.NewTimer(time.Millisecond * time.Duration((first-idx)*100)).C
		}
	}

	return err
}

func NewAccessModel(modelXml string, saveXml string, totalCountPerDay int) *AccessModel {
	return &AccessModel{
		modelXml:         modelXml,
		saveXml:          saveXml,
		totalCountPerDay: totalCountPerDay,
	}
}

func (am *AccessModel) Request() error {
	for {
		now := time.Now()
		dateString := now.Format("2006-01-02")

		err := func() error {
			am.mutex.Lock()
			defer am.mutex.Unlock()

			if am.inner == nil || am.inner.date != dateString {
				inner, err := am.loadInnerAccessModel(now)
				if err != nil {
					return err
				}

				am.inner = inner

				err = am.save()
				if err != nil {
					return err
				}
			}

			return nil
		}()
		if err != nil {
			return err
		}

		err = am.inner.request(now)
		if err == errorRetry {
			continue
		}

		return err
	}
}

func (am *AccessModel) loadInnerAccessModel(now time.Time) (*InnerAccessModel, error) {
	data, err := ioutil.ReadFile(am.modelXml)
	if err != nil {
		return nil, err
	}
	aModel := AModel{}
	err = xml.Unmarshal(data, &aModel)
	if err != nil {
		return nil, err
	}

	if aModel.DeviationPerHour >= 0.9 || aModel.DeviationPerDay >= 0.9 {
		return nil, fmt.Errorf("DeviationPerHour or DeviationPerDay must less than 0.9")
	}

	sa := aModel.sumAccesses()
	if sa <= 0.1 {
		return nil, fmt.Errorf("sumAccesses error")
	}

	dateString := now.Format("2006-01-02")

	saveXml := fmt.Sprintf("%s.%s.xml", am.saveXml, dateString)

	exists, err := am.pathExists(saveXml)
	if err != nil {
		return nil, err
	}
	if exists {
		data, err = ioutil.ReadFile(saveXml)
		if err != nil {
			return nil, err
		}

		inner := InnerAccessModel{}
		err = xml.Unmarshal(data, &inner)
		if err != nil {
			return nil, err
		}

		sa := inner.AModel.sumAccesses()
		if sa <= 0.1 {
			panic(fmt.Errorf("sumAccesses error"))
		}

		inner.outer = am
		inner.date = dateString
		inner.saveXml = saveXml

		inner.calcModels()

		return &inner, nil
	}

	inner := InnerAccessModel{
		AModel:  &aModel,
		outer:   am,
		date:    dateString,
		saveXml: saveXml,
	}

	inner.random()

	return &inner, nil
}

func (am *AccessModel) save() error {
	if am.inner == nil {
		return nil
	}

	data, err := xml.MarshalIndent(am.inner, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(am.inner.saveXml, data, 0777)
}

func (am *AccessModel) pathExists(path string) (bool, error) {
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
