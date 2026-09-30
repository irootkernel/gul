package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
	"github.com/rootkernel/gul/internal/workspace/contractprovider"
)

type picker struct{ path string }

func (p picker) PickDirectory(context.Context) (string, error) { return p.path, nil }

type inspectionCall struct {
	path     string
	expected *string
}

type recordingProvider struct {
	base  workspace.Provider
	calls []inspectionCall
}

func (p *recordingProvider) InspectWorkspace(ctx context.Context, path string, expected *string) (workspace.Inspection, error) {
	p.calls = append(p.calls, inspectionCall{path: path, expected: expected})
	return p.base.InspectWorkspace(ctx, path, expected)
}

func testRepository(t *testing.T) workspace.Repository {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	return store.Presentation()
}

func TestAllowlistPickerAndCanonicalRevalidation(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	canonicalProject, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	canonicalProject = filepath.Join(canonicalProject, "project")
	outside := t.TempDir()
	for _, dir := range []string{project, filepath.Join(root, ".dolgorae")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(project, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{base: contractprovider.ContractProvider{Port: scenario.New(time.Now())}}
	repo := testRepository(t)
	service := workspace.NewService(provider, repo, picker{path: filepath.Join(root, "alias")}, []string{root})
	if got := service.ListRegistrableRoots(); len(got) != 1 || got[0].ID != "root-1" {
		t.Fatalf("roots = %+v", got)
	}
	options, err := service.BrowseRegistrableRoot("root-1", ".")
	if err != nil || !reflect.DeepEqual(options, []workspace.DirectoryOption{{Name: "project", RelativePath: "project"}}) {
		t.Fatalf("browse = %+v, %v", options, err)
	}
	for _, relative := range []string{"../outside", "/absolute", "escape", "alias", ".dolgorae", ".DOLGORAE", "project/../escape", "project\\escape"} {
		if _, err := service.BrowseRegistrableRoot("root-1", relative); !errors.Is(err, workspace.ErrSelectionUnavailable) {
			t.Errorf("browse %q = %v", relative, err)
		}
		if _, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", relative); !errors.Is(err, workspace.ErrSelectionUnavailable) {
			t.Errorf("register %q = %v", relative, err)
		}
	}
	entry, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
	if err != nil || entry.CanonicalRoot != canonicalProject || entry.ProviderID == "" || entry.ID == "" {
		t.Fatalf("attachment = %+v, %v", entry, err)
	}
	if len(provider.calls) != 1 || provider.calls[0].expected != nil || provider.calls[0].path != canonicalProject {
		t.Fatalf("bootstrap calls = %+v", provider.calls)
	}
	duplicate, err := service.RegisterFromHostSelection(t.Context(), "owner")
	if err != nil || duplicate != entry {
		t.Fatalf("picker alias attachment = %+v, %v", duplicate, err)
	}
	var group sync.WaitGroup
	concurrentService := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, repo, nil, []string{root})
	for range 8 {
		group.Go(func() {
			concurrent, err := concurrentService.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
			if err != nil || concurrent.ID != entry.ID {
				t.Errorf("concurrent attachment = %+v, %v", concurrent, err)
			}
		})
	}
	group.Wait()
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); err != nil {
		t.Fatal(err)
	}
	last := provider.calls[len(provider.calls)-1]
	if last.path != canonicalProject || last.expected == nil || *last.expected != entry.ProviderID {
		t.Fatalf("revalidation call = %+v", last)
	}
	provider.base = fixedProvider{workspace.Inspection{CanonicalRoot: canonicalProject, ProviderID: entry.ProviderID, Blocker: workspace.BlockerProfileServerUnavailable}}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); !errors.Is(err, workspace.ErrProfileServerUnavailable) {
		t.Fatalf("provider availability = %v", err)
	}
	provider.base = fixedProvider{workspace.Inspection{CanonicalRoot: canonicalProject, ProviderID: "different", Compatible: true}}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); !errors.Is(err, workspace.ErrReattachRequired) {
		t.Fatalf("changed provider identity = %v", err)
	}
	if err := os.Rename(project, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); !errors.Is(err, workspace.ErrReattachRequired) {
		t.Fatalf("replaced directory = %v", err)
	}
	moved := filepath.Join(root, "moved")
	canonicalMoved := filepath.Join(filepath.Dir(canonicalProject), "moved")
	provider.base = fixedProvider{workspace.Inspection{CanonicalRoot: moved, ProviderID: entry.ProviderID, Compatible: true}}
	reattached, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "moved")
	if err != nil || reattached.ID != entry.ID || reattached.CanonicalRoot != canonicalMoved {
		t.Fatalf("moved workspace reattachment = %+v, %v", reattached, err)
	}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); err != nil {
		t.Fatalf("reattached workspace = %v", err)
	}
	if err := os.Remove(moved); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); !errors.Is(err, workspace.ErrReattachRequired) {
		t.Fatalf("missing canonical root = %v", err)
	}
	if err := os.Mkdir(moved, 0700); err != nil {
		t.Fatal(err)
	}
	provider.base = fixedProvider{workspace.Inspection{CanonicalRoot: moved, ProviderID: "replacement", Compatible: true}}
	replaced, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "moved")
	if err != nil || replaced.ID != entry.ID || replaced.ProviderID != "replacement" {
		t.Fatalf("replaced workspace reattachment = %+v, %v", replaced, err)
	}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); err != nil {
		t.Fatalf("replacement workspace = %v", err)
	}
}

func TestRegistrationDoesNotCreateWorktreesOrModifyWorkspace(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git", "refs", "heads"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"README.md":   "existing workspace\n",
		".git/HEAD":   "ref: refs/heads/main\n",
		".git/config": "[core]\n\tbare = false\n",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	type fileState struct {
		mode     os.FileMode
		modified time.Time
		content  string
	}
	snapshot := func() map[string]fileState {
		t.Helper()
		files := make(map[string]fileState)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			state := fileState{mode: info.Mode(), modified: info.ModTime()}
			if !entry.IsDir() {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				state.content = string(content)
			}
			files[path] = state
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	service := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())},
		testRepository(t), picker{path: project}, []string{root})
	entry, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegisterFromHostSelection(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revalidate(t.Context(), "owner", entry.ID); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("registration or revalidation changed the workspace tree, Git metadata, or surrounding directory:\nbefore: %+v\nafter: %+v", before, after)
	}
}

func TestConcurrentFirstAttachmentCreatesOneEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "project"), 0700); err != nil {
		t.Fatal(err)
	}
	repo := testRepository(t)
	service := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, repo, nil, []string{root})
	var group sync.WaitGroup
	ids := make(chan string, 8)
	for range cap(ids) {
		group.Go(func() {
			entry, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
			if err != nil {
				t.Errorf("concurrent first registration: %v", err)
				return
			}
			ids <- entry.ID
		})
	}
	group.Wait()
	close(ids)
	var first string
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Errorf("different attachment IDs: %q and %q", first, id)
		}
	}
	entries, err := repo.ListAttachments(t.Context(), "owner")
	if err != nil || len(entries) != 1 || entries[0].ID != first {
		t.Fatalf("concurrent first registration stored %+v, %v", entries, err)
	}
}

type barrierRepository struct {
	workspace.Repository
	mu      sync.Mutex
	reads   int
	barrier chan struct{}
}

func (r *barrierRepository) ListAttachments(ctx context.Context, subject string) ([]workspace.Attachment, error) {
	entries, err := r.Repository.ListAttachments(ctx, subject)
	r.mu.Lock()
	r.reads++
	read := r.reads
	if read == 2 {
		close(r.barrier)
	}
	r.mu.Unlock()
	if read <= 2 {
		<-r.barrier
	}
	return entries, err
}

func TestConcurrentServicesReconcileUniqueAttachment(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "project"), 0700); err != nil {
		t.Fatal(err)
	}
	repo := &barrierRepository{Repository: testRepository(t), barrier: make(chan struct{})}
	provider := contractprovider.ContractProvider{Port: scenario.New(time.Now())}
	services := []*workspace.Service{
		workspace.NewService(provider, repo, nil, []string{root}),
		workspace.NewService(provider, repo, nil, []string{root}),
	}
	ids := make(chan string, 2)
	var group sync.WaitGroup
	for _, service := range services {
		group.Go(func() {
			entry, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
			if err != nil {
				t.Errorf("cross-service registration: %v", err)
				return
			}
			ids <- entry.ID
		})
	}
	group.Wait()
	close(ids)
	var first string
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Errorf("different attachment IDs: %q and %q", first, id)
		}
	}
	entries, err := repo.Repository.ListAttachments(t.Context(), "owner")
	if err != nil || len(entries) != 1 || entries[0].ID != first {
		t.Fatalf("cross-service attachment = %+v, %v", entries, err)
	}
}

func TestStalePathAliasDoesNotBlockAttachedWorkspace(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	service := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, testRepository(t), nil, []string{root})
	for _, name := range []string{"a", "b"} {
		if _, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", name); err != nil {
			t.Fatal(err)
		}
	}
	attached, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "b")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "b"), filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	again, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "b")
	if err != nil || again.ID != attached.ID {
		t.Fatalf("stale alias interfered with attached workspace: %+v, %v", again, err)
	}
}

func TestAttachmentsAreSubjectScoped(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "project"), 0700); err != nil {
		t.Fatal(err)
	}
	repo := testRepository(t)
	service := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, repo, nil, []string{root})
	owner, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
	if err != nil {
		t.Fatal(err)
	}
	otherList, err := service.List(t.Context(), "other")
	if err != nil || len(otherList) != 0 {
		t.Fatalf("other subject sees owner attachment: %+v, %v", otherList, err)
	}
	if _, err := service.Revalidate(t.Context(), "other", owner.ID); !errors.Is(err, workspace.ErrReattachRequired) {
		t.Fatalf("foreign attachment revalidation = %v", err)
	}
	other, err := service.RegisterFromAllowlistPath(t.Context(), "other", "root-1", "project")
	if err == nil || other.ID != "" {
		t.Fatalf("unregistered subject created an attachment = %+v, %v", other, err)
	}
}

type fixedProvider struct{ inspection workspace.Inspection }

func (p fixedProvider) InspectWorkspace(context.Context, string, *string) (workspace.Inspection, error) {
	return p.inspection, nil
}

func TestAllowlistAcceptsHostCaseAlias(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	alias := strings.ToUpper(canonical)
	originalInfo, originalErr := os.Stat(canonical)
	aliasInfo, aliasErr := os.Stat(alias)
	if originalErr != nil || aliasErr != nil || !os.SameFile(originalInfo, aliasInfo) {
		t.Skip("test volume has no case-insensitive alias")
	}
	service := workspace.NewService(fixedProvider{workspace.Inspection{CanonicalRoot: alias, ProviderID: "provider", Compatible: true}}, testRepository(t), nil, []string{root})
	entry, err := service.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project")
	if err != nil || entry.CanonicalRoot != alias {
		t.Fatalf("case-alias attachment = %+v, %v", entry, err)
	}
}

func TestProviderMismatchAndUnavailableAllowlist(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	repo := testRepository(t)
	blocked := workspace.NewService(fixedProvider{workspace.Inspection{CanonicalRoot: project, ProviderID: "id"}}, repo, picker{project}, []string{filepath.Join(root, "absent")})
	if len(blocked.ListRegistrableRoots()) != 0 {
		t.Fatal("unresolvable allowlist remained available")
	}
	if _, err := blocked.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "."); !errors.Is(err, workspace.ErrHostSelectionUnavailable) {
		t.Fatalf("disabled browse = %v", err)
	}
	if !errors.Is(blocked.RootConfigurationError(), workspace.ErrHostSelectionUnavailable) {
		t.Fatal("missing host allowlist diagnostic")
	}
	if _, err := blocked.RegisterFromHostSelection(t.Context(), "owner"); !errors.Is(err, workspace.ErrWorkspaceBlocked) {
		t.Fatalf("host picker provider blocker = %v", err)
	}
	availablePicker := workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, repo, picker{project}, []string{filepath.Join(root, "absent")})
	if entry, err := availablePicker.RegisterFromHostSelection(t.Context(), "owner"); err != nil || entry.ID == "" {
		t.Fatalf("host picker with unavailable browse = %+v, %v", entry, err)
	}
	wrong := workspace.NewService(fixedProvider{workspace.Inspection{CanonicalRoot: t.TempDir(), ProviderID: "id", Compatible: true}}, repo, picker{project}, nil)
	if _, err := wrong.RegisterFromHostSelection(t.Context(), "owner"); !errors.Is(err, workspace.ErrIdentityMismatch) {
		t.Fatalf("provider path mismatch = %v", err)
	}
	outside := t.TempDir()
	wrongAllowlist := workspace.NewService(fixedProvider{workspace.Inspection{CanonicalRoot: outside, ProviderID: "outside", Compatible: true}}, repo, nil, []string{root})
	if _, err := wrongAllowlist.RegisterFromAllowlistPath(t.Context(), "owner", "root-1", "project"); !errors.Is(err, workspace.ErrIdentityMismatch) {
		t.Fatalf("provider returned outside root = %v", err)
	}
	entries, err := repo.ListAttachments(t.Context(), "owner")
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected attachments = %+v, %v", entries, err)
	}
}

func TestProviderBlockersRemainTyped(t *testing.T) {
	project := t.TempDir()
	for _, test := range []struct {
		blocker workspace.Blocker
		want    error
	}{
		{workspace.BlockerUninitialized, workspace.ErrWorkspaceUninitialized},
		{workspace.BlockerProfileMissing, workspace.ErrProfileMissing},
		{workspace.BlockerProfileServerUnavailable, workspace.ErrProfileServerUnavailable},
	} {
		t.Run(test.want.Error(), func(t *testing.T) {
			service := workspace.NewService(fixedProvider{workspace.Inspection{CanonicalRoot: project, ProviderID: "id", Blocker: test.blocker}}, testRepository(t), picker{project}, nil)
			if _, err := service.RegisterFromHostSelection(t.Context(), "owner"); !errors.Is(err, test.want) {
				t.Fatalf("blocker = %v, want %v", err, test.want)
			}
		})
	}
}
