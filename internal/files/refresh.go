package files

import "context"

func revisionKey(subject, workspaceID string) string { return subject + "\x00" + workspaceID }

// Refresh invalidates retained directory pages and gives clients a generation
// to compare before re-reading. Content reads are already uncached.
func (s *Service) Refresh(ctx context.Context, subject, workspaceID string) (uint64, error) {
	root, _, err := s.Open(ctx, subject, workspaceID, ".")
	if err != nil {
		return 0, err
	}
	root.Close()
	s.cursorMu.Lock()
	defer s.cursorMu.Unlock()
	for token, cursor := range s.cursors {
		if cursor.subject == subject && cursor.workspaceID == workspaceID {
			cursor.dir.Close()
			delete(s.cursors, token)
		}
	}
	if s.revisions == nil {
		s.revisions = make(map[string]uint64)
	}
	key := revisionKey(subject, workspaceID)
	s.revisions[key]++
	return s.revisions[key], nil
}
