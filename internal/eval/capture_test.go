package eval

import (
	"reflect"
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestExpectedReviewCaptureIDs(t *testing.T) {
	for _, test := range []struct {
		name   string
		review map[string]any
		passes int
		want   []string
	}{
		{
			name:   "single unsplit execution",
			review: map[string]any{"id": "review-1"},
			want:   []string{"review-1"},
		},
		{
			name: "planner fallback plus unsplit review",
			review: map[string]any{
				"id":     "review-2",
				"shards": map[string]any{"planner": "one feature"},
			},
			want: []string{"review-2", "review-2-plan"},
		},
		{
			name: "planned shards and boundary",
			review: map[string]any{
				"id": "review-3",
				"shards": map[string]any{
					"planner":  "planned",
					"shards":   []any{map[string]any{"executions": []any{"shard-1", "shard-2"}}},
					"boundary": map[string]any{"executions": []any{"boundary-1"}},
				},
			},
			want: []string{"boundary-1", "review-3-plan", "shard-1", "shard-2"},
		},
		{
			name:   "multiple unsplit passes",
			review: map[string]any{"id": "review-4"},
			passes: 2,
			want:   []string{"review-4-pass-1", "review-4-pass-2"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := expectedReviewCaptureIDs(test.review, test.passes); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("expectedReviewCaptureIDs() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestReviewCapturesVerifiedComparesEveryExecutionInput(t *testing.T) {
	plannerInput := []byte(`{"Intent":"plan"}`)
	shardInput := []byte(`{"Paths":["a.go"]}`)
	boundaryInput := []byte(`{"Paths":["a.go","b.go"]}`)
	review := map[string]any{
		"id": "review-1",
		"input_digests": map[string]any{
			"review-1-plan": runner.BytesDigest(plannerInput),
			"shard-1":       runner.BytesDigest(shardInput),
			"boundary-1":    runner.BytesDigest(boundaryInput),
		},
		"shards": map[string]any{
			"planner":  "planned",
			"shards":   []any{map[string]any{"executions": []any{"shard-1"}}},
			"boundary": map[string]any{"executions": []any{"boundary-1"}},
		},
	}
	captures := []reviewCapture{
		{ID: "review-1-plan", Input: plannerInput, HasInput: true, HasArgv: true, HasEvents: true},
		{ID: "shard-1", Input: shardInput, HasInput: true, HasArgv: true, HasEvents: true},
		{ID: "boundary-1", Input: boundaryInput, HasInput: true, HasArgv: true, HasEvents: true},
	}
	if !reviewCapturesVerified(review, captures, 1, nil) {
		t.Fatal("matching per-execution inputs were not verified")
	}
	captures[2].Input = shardInput
	if reviewCapturesVerified(review, captures, 1, nil) {
		t.Fatal("swapped boundary input was marked verified")
	}
}

func TestReviewCapturesVerifiedRequiresEveryExecution(t *testing.T) {
	plannerInput, shardInput, boundaryInput := []byte("planner"), []byte("shard"), []byte("boundary")
	review := map[string]any{
		"id": "review-1",
		"input_digests": map[string]any{
			"review-1-plan": runner.BytesDigest(plannerInput),
			"shard-1":       runner.BytesDigest(shardInput),
			"boundary-1":    runner.BytesDigest(boundaryInput),
		},
		"shards": map[string]any{
			"planner":  "planned",
			"shards":   []any{map[string]any{"executions": []any{"shard-1"}}},
			"boundary": map[string]any{"executions": []any{"boundary-1"}},
		},
	}
	captures := []reviewCapture{
		{ID: "review-1-plan", Input: plannerInput, HasInput: true, HasArgv: true, HasEvents: true},
		{ID: "shard-1", Input: shardInput, HasInput: true, HasArgv: true, HasEvents: true},
		{ID: "boundary-1", Input: boundaryInput, HasInput: true, HasArgv: true, HasEvents: true},
	}
	if !reviewCapturesVerified(review, captures, 1, nil) {
		t.Fatal("complete sharded execution captures were not verified")
	}
	captures[2].HasEvents = false
	if reviewCapturesVerified(review, captures, 1, nil) {
		t.Fatal("incomplete boundary capture was marked verified")
	}
}
