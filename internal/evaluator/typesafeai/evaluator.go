// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package typesafeai implements the evaluator backend using the TypeSafe AI
// System One API (https://typesafe.ai) and its "jev" model.
//
// All criteria for a subject are sent as questions of a single request, with
// the subject code as the request state. Each criterion maps to a Score
// question whose criteria are the levels.
package typesafeai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/chez-shanpu/typesafeai-go"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
)

// BackendName is the name this backend is registered under.
const BackendName = "typesafeai"

const apiKeyEnvVar = "TYPESAFE_API_KEY"

// language is the language of every subject; tastecheck only checks Go code.
const language = "go"

func init() {
	evaluator.Register(BackendName, func(opts evaluator.Options) (evaluator.Evaluator, error) {
		apiKey := os.Getenv(apiKeyEnvVar)
		if apiKey == "" {
			return nil, fmt.Errorf("%s is not set", apiKeyEnvVar)
		}
		client := typesafeai.NewSystemOneClient(apiKey, http.DefaultClient)
		return newEvaluator(client, opts.Model), nil
	})
}

// state is the System One request state: the code under evaluation.
type state struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type evaluatorImpl struct {
	client *typesafeai.SystemOneClient
	model  string
}

func newEvaluator(client *typesafeai.SystemOneClient, model string) *evaluatorImpl {
	return &evaluatorImpl{
		client: client,
		model:  cmp.Or(model, typesafeai.ModelJevLatest),
	}
}

// Evaluate implements evaluator.Evaluator.
func (e *evaluatorImpl) Evaluate(ctx context.Context, s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
	// Question keys are positional rather than criterion IDs, so that the
	// request does not depend on which characters the API accepts in keys.
	questions := make(map[string]typesafeai.Question, len(cs))
	for i, c := range cs {
		questions[questionKey(i)] = &typesafeai.ScoreQuestion{Instructions: c.Description, Criteria: c.Levels}
	}

	resp, err := e.client.Do(ctx, &typesafeai.SystemOneRequest{
		State:     state{Language: language, Code: s.Code},
		Model:     e.model,
		Questions: questions,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate %s: %w", s.ID, classify(ctx, err))
	}

	verdicts := make([]evaluator.Verdict, 0, len(cs))
	for i, c := range cs {
		v, err := toVerdict(c, resp.Answers[questionKey(i)])
		if err != nil {
			return nil, fmt.Errorf("evaluate %s: %s: %w", s.ID, c.ID, err)
		}
		verdicts = append(verdicts, v)
	}
	return verdicts, nil
}

func questionKey(i int) string {
	return "q" + strconv.Itoa(i)
}

func toVerdict(c evaluator.Criterion, ans typesafeai.Answer) (evaluator.Verdict, error) {
	switch a := ans.(type) {
	case nil:
		return evaluator.Verdict{}, errors.New("answer is missing")
	case *typesafeai.ScoreAnswer:
		return scoreVerdict(c, a), nil
	default:
		return evaluator.Verdict{}, fmt.Errorf("unexpected answer type %T", ans)
	}
}

// scoreVerdict normalizes the probability-weighted level index (0 = worst)
// into [0, 1]. The label is the level nearest to that weighted index rather
// than the most probable level, so that it stays consistent with the score
// used for pass/fail even when the probabilities are bimodal.
func scoreVerdict(c evaluator.Criterion, a *typesafeai.ScoreAnswer) evaluator.Verdict {
	top := len(c.Levels) - 1
	level := min(max(int(math.Round(a.Score)), 0), top)
	v := evaluator.Verdict{
		CriterionID: c.ID,
		Score:       min(max(a.Score/float64(top), 0), 1),
		Confidence:  new(a.Confidence),
		Label:       c.Levels[level],
	}
	if level < top {
		v.NextLabel = c.Levels[level+1]
	}
	return v
}

// classify wraps errors that are worth retrying with evaluator.ErrTransient.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return err
	}
	if apiErr, ok := errors.AsType[*typesafeai.APIError](err); ok {
		if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("%w: %w", evaluator.ErrTransient, err)
		}
		return err
	}
	if _, ok := errors.AsType[*typesafeai.ConnectionError](err); ok {
		return fmt.Errorf("%w: %w", evaluator.ErrTransient, err)
	}
	return err
}
