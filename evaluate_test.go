package wemdigo

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvaluateEval(t *testing.T) {
	f := func() error {
		return nil
	}
	e := &evaluate{f: f}
	e.eval()
	err := <-e.err
	assert.NoError(t, err)
}
