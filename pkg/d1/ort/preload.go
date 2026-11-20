package ort

import (
	"errors"
	"fmt"
)

func preload(paths []string, open func(string) error) error {
	pending := paths
	for len(pending) > 0 {
		var failed []string
		var errs []error
		for _, p := range pending {
			if err := open(p); err != nil {
				failed = append(failed, p)
				errs = append(errs, fmt.Errorf("%s: %w", p, err))
			}
		}
		if len(failed) == len(pending) {
			return errors.Join(errs...)
		}
		pending = failed
	}
	return nil
}
