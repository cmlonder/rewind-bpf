package runid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func New() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "run_" + time.Now().UTC().Format("20060102T150405.000000000Z") + "_" + hex.EncodeToString(suffix[:]), nil
}
