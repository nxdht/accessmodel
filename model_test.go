package accessmodel

import (
	"testing"
	"time"
)

func TestModel1(t *testing.T) {
	am := NewAccessModel("model.xml", "save.xml", 100000)

	for k := 0; k < 3; k++ {
		go func(k int) {
			for i := 0; i < 10; i++ {
				err := am.Request()
				if err != nil {
					t.Error(err)
				}
				t.Logf("%d: %d", k, time.Now().UnixNano()/1000000)
			}
		}(k)
	}
	for i := 0; i < 10; i++ {
		err := am.Request()
		if err != nil {
			t.Error(err)
		}
		t.Logf("%d", time.Now().UnixNano()/1000000)
	}
}
