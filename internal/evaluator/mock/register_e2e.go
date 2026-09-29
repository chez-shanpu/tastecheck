//go:build e2e

// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package mock

import (
	"strings"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
)

// The e2e tests build the binary with the e2e tag to get the "mock" backend,
// which scores functions whose source contains "Bad" as 0 and all other
// functions as 1, without calling any API.
func init() {
	evaluator.Register("mock", func(evaluator.Options) (evaluator.Evaluator, error) {
		return &Evaluator{Func: func(s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
			score := 1.0
			if strings.Contains(s.Code, "Bad") {
				score = 0
			}
			vs := make([]evaluator.Verdict, 0, len(cs))
			for _, c := range cs {
				vs = append(vs, evaluator.Verdict{CriterionID: c.ID, Score: score, Label: "label"})
			}
			return vs, nil
		}}, nil
	})
}
