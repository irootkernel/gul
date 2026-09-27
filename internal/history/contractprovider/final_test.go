package contractprovider

import (
	"errors"
	"strings"
	"testing"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/history"
)

func TestSnapshotRejectsOversizedOrInvalidInlineFinal(t *testing.T) {
	p, w, b := fixture(t)
	for _, value := range []string{strings.Repeat("x", int(p.maxInline)+1), string([]byte{0xff})} {
		w.run.LastFinalResponse = &publicv1.FinalResponse{Value: &publicv1.FinalResponse_InlineUtf8{InlineUtf8: value}}
		if _, err := p.Snapshot(t.Context(), b); !errors.Is(err, history.ErrLimit) {
			t.Fatal("accepted invalid inline final", err)
		}
	}
}

func TestSnapshotExposesCheckedFinalForRecoveryRefresh(t *testing.T) {
	p, w, b := fixture(t)
	w.run.LastFinalResponse = &publicv1.FinalResponse{Value: &publicv1.FinalResponse_InlineUtf8{InlineUtf8: "final\r\n"}}
	s, err := p.Snapshot(t.Context(), b)
	if err != nil || s.Final == nil || s.Final.Inline == nil || *s.Final.Inline != "final\r\n" {
		t.Fatal(s, err)
	}
	w.artifact.Kind = publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE
	w.run.LastFinalResponse = &publicv1.FinalResponse{Value: &publicv1.FinalResponse_Artifact{Artifact: w.artifact}}
	s, err = p.Snapshot(t.Context(), b)
	if err != nil || s.Final == nil || s.Final.Artifact == nil || s.Final.Artifact.ID != w.artifact.ArtifactId {
		t.Fatal(s, err)
	}
	w.artifact.Kind = publicv1.ArtifactKind_ARTIFACT_KIND_USER_INPUT
	if _, err = p.Snapshot(t.Context(), b); !errors.Is(err, history.ErrBlocked) {
		t.Fatal("wrong final artifact kind", err)
	}
	w.run.LastFinalResponse = &publicv1.FinalResponse{Value: &publicv1.FinalResponse_Unavailable{Unavailable: &publicv1.UnavailableContent{Reason: "not available"}}}
	s, err = p.Snapshot(t.Context(), b)
	if err != nil || !s.FinalUnavailable || s.Final != nil {
		t.Fatal("unavailable final lost", s, err)
	}
}
