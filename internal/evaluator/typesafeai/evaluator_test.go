// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package typesafeai

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/chez-shanpu/typesafeai-go"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newStubEvaluator returns an evaluator backed by a real SystemOneClient whose
// transport records the request body and replies with status and body.
func newStubEvaluator(t *testing.T, status int, body string) (*evaluatorImpl, *[]byte) {
	t.Helper()
	var reqBody []byte
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		reqBody = b
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	return newEvaluator(typesafeai.NewSystemOneClient("test-key", hc), ""), &reqBody
}

var (
	srpScore = evaluator.Criterion{
		ID:          "uber-go/srp",
		Description: "single responsibility",
		Levels:      []string{"many", "two", "mostly", "single"},
	}
	srpYesNo = evaluator.Criterion{
		ID:          "srp-yes-no",
		Description: "is it single?",
		Levels:      []string{"multiple", "single"},
	}
	subject = evaluator.Subject{ID: "a.go:1 F", Code: "func F() {}"}
)

func TestEvaluateRequest(t *testing.T) {
	e, reqBody := newStubEvaluator(t, http.StatusOK, `{
		"model": "jev-1",
		"answers": {
			"q0": {"type": "score", "score": 3, "legend": {}, "probabilities": {"3": 1}, "confidence": 1},
			"q1": {"type": "score", "score": 0.9, "legend": {}, "probabilities": {"0": 0.1, "1": 0.9}, "confidence": 0.8}
		},
		"usage": {"input_tokens": 1, "output_tokens": 1}
	}`)

	verdicts, err := e.Evaluate(t.Context(), subject, []evaluator.Criterion{srpScore, srpYesNo})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	// Answers are mapped back to criteria by position.
	if len(verdicts) != 2 ||
		verdicts[0].CriterionID != "uber-go/srp" || verdicts[0].Score != 1 ||
		verdicts[1].CriterionID != "srp-yes-no" || verdicts[1].Score != 0.9 {
		t.Errorf("verdicts = %+v", verdicts)
	}

	var got map[string]any
	if err := json.Unmarshal(*reqBody, &got); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	want := map[string]any{
		"state": map[string]any{"language": "go", "code": "func F() {}"},
		"model": typesafeai.ModelJevLatest,
		"questions": map[string]any{
			"q0": map[string]any{
				"type":         "score",
				"instructions": "single responsibility",
				"criteria":     []any{"many", "two", "mostly", "single"},
			},
			"q1": map[string]any{
				"type":         "score",
				"instructions": "is it single?",
				"criteria":     []any{"multiple", "single"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v", got, want)
	}
}

func TestEvaluateVerdicts(t *testing.T) {
	tests := []struct {
		name     string
		criteria []evaluator.Criterion
		answers  string
		want     []evaluator.Verdict
	}{
		{
			name:     "score is normalized and labeled by nearest level",
			criteria: []evaluator.Criterion{srpScore},
			answers:  `"q0": {"type": "score", "score": 1.125, "legend": {}, "probabilities": {"0": 0.1, "1": 0.675, "2": 0.225}, "confidence": 0.6}`,
			want: []evaluator.Verdict{{
				CriterionID: "uber-go/srp", Score: 0.375, Confidence: new(0.6), Label: "two", NextLabel: "mostly",
			}},
		},
		{
			name:     "bimodal probabilities are labeled consistently with the score",
			criteria: []evaluator.Criterion{srpScore},
			answers:  `"q0": {"type": "score", "score": 1.45, "legend": {}, "probabilities": {"0": 0.35, "1": 0.25, "3": 0.4}, "confidence": 0.3}`,
			want: []evaluator.Verdict{{
				CriterionID: "uber-go/srp", Score: 1.45 / 3, Confidence: new(0.3), Label: "two", NextLabel: "mostly",
			}},
		},
		{
			name:     "best score has no next label",
			criteria: []evaluator.Criterion{srpScore},
			answers:  `"q0": {"type": "score", "score": 3, "legend": {}, "probabilities": {"3": 1}, "confidence": 1}`,
			want: []evaluator.Verdict{{
				CriterionID: "uber-go/srp", Score: 1, Confidence: new(1.0), Label: "single",
			}},
		},
		{
			name:     "out of range score is clamped",
			criteria: []evaluator.Criterion{srpScore},
			answers:  `"q0": {"type": "score", "score": 3.2, "legend": {}, "probabilities": {}, "confidence": 1}`,
			want: []evaluator.Verdict{{
				CriterionID: "uber-go/srp", Score: 1, Confidence: new(1.0), Label: "single",
			}},
		},
		{
			name:     "zero confidence is reported rather than treated as missing",
			criteria: []evaluator.Criterion{srpScore},
			answers:  `"q0": {"type": "score", "score": 1.5, "legend": {}, "probabilities": {"0": 0.5, "3": 0.5}, "confidence": 0}`,
			want: []evaluator.Verdict{{
				CriterionID: "uber-go/srp", Score: 0.5, Confidence: new(0.0), Label: "mostly", NextLabel: "single",
			}},
		},
		{
			name:     "two levels score the probability of the better one",
			criteria: []evaluator.Criterion{srpYesNo},
			answers:  `"q0": {"type": "score", "score": 0.8, "legend": {}, "probabilities": {"0": 0.2, "1": 0.8}, "confidence": 0.6}`,
			want:     []evaluator.Verdict{{CriterionID: "srp-yes-no", Score: 0.8, Confidence: new(0.6), Label: "single"}},
		},
		{
			name:     "two levels below half are labeled by the worse one",
			criteria: []evaluator.Criterion{srpYesNo},
			answers:  `"q0": {"type": "score", "score": 0.2, "legend": {}, "probabilities": {"0": 0.8, "1": 0.2}, "confidence": 0.6}`,
			want:     []evaluator.Verdict{{CriterionID: "srp-yes-no", Score: 0.2, Confidence: new(0.6), Label: "multiple", NextLabel: "single"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newStubEvaluator(t, http.StatusOK, `{"model": "jev-1", "answers": {`+tt.answers+`}, "usage": {}}`)
			got, err := e.Evaluate(t.Context(), subject, tt.criteria)
			if err != nil {
				t.Fatalf("Evaluate() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Evaluate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvaluateErrors(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		wantTransient bool
		wantErr       string
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{}`, wantTransient: true},
		{name: "overloaded", status: 529, body: `{}`, wantTransient: true},
		{name: "server error", status: http.StatusInternalServerError, body: `{}`, wantTransient: true},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{}`, wantErr: "status 401"},
		{name: "invalid request", status: http.StatusUnprocessableEntity, body: `{}`, wantErr: "status 422"},
		{
			name:    "missing answer",
			status:  http.StatusOK,
			body:    `{"model": "jev-1", "answers": {}, "usage": {}}`,
			wantErr: "answer is missing",
		},
		{
			name:    "non-score answer",
			status:  http.StatusOK,
			body:    `{"model": "jev-1", "answers": {"q0": {"type": "noul", "noul": 1}}, "usage": {}}`,
			wantErr: "unexpected answer type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newStubEvaluator(t, tt.status, tt.body)
			_, err := e.Evaluate(t.Context(), subject, []evaluator.Criterion{srpScore})
			if err == nil {
				t.Fatal("Evaluate() expected error")
			}
			if got := errors.Is(err, evaluator.ErrTransient); got != tt.wantTransient {
				t.Errorf("errors.Is(err, ErrTransient) = %v, want %v (err: %v)", got, tt.wantTransient, err)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Evaluate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateConnectionError(t *testing.T) {
	hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	e := newEvaluator(typesafeai.NewSystemOneClient("test-key", hc), "")

	_, err := e.Evaluate(t.Context(), subject, []evaluator.Criterion{srpScore})
	if !errors.Is(err, evaluator.ErrTransient) {
		t.Errorf("Evaluate() error = %v, want ErrTransient", err)
	}
}

func TestFactory(t *testing.T) {
	t.Setenv(apiKeyEnvVar, "")
	if _, err := evaluator.New(BackendName, evaluator.Options{}); err == nil {
		t.Error("New() expected error without API key")
	}

	t.Setenv(apiKeyEnvVar, "key")
	e, err := evaluator.New(BackendName, evaluator.Options{Model: "jev-custom"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := e.(*evaluatorImpl).model; got != "jev-custom" {
		t.Errorf("model = %q, want jev-custom", got)
	}
}
