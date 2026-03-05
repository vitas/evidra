package evidence

import "testing"

func TestAppendEntryAtPath_WritesThroughLookupCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	entry := buildTestEntry(t, EntryTypePrescribe, "")

	resetLookupCacheForPath(dir)
	if err := AppendEntryAtPath(dir, entry); err != nil {
		t.Fatalf("AppendEntryAtPath: %v", err)
	}

	cached, ok := lookupCachedEntryByID(dir, entry.EntryID)
	if !ok {
		t.Fatal("expected entry in lookup cache after append")
	}
	if cached.EntryID != entry.EntryID {
		t.Fatalf("cached entry_id=%q, want %q", cached.EntryID, entry.EntryID)
	}
}

func TestFindEntryByID_PopulatesLookupCacheOnScan(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	entry := buildTestEntry(t, EntryTypePrescribe, "")
	if err := AppendEntryAtPath(dir, entry); err != nil {
		t.Fatalf("AppendEntryAtPath: %v", err)
	}

	resetLookupCacheForPath(dir)
	if _, ok := lookupCachedEntryByID(dir, entry.EntryID); ok {
		t.Fatal("expected empty cache after reset")
	}

	got, found, err := FindEntryByID(dir, entry.EntryID)
	if err != nil {
		t.Fatalf("FindEntryByID: %v", err)
	}
	if !found {
		t.Fatal("expected entry to be found")
	}
	if got.EntryID != entry.EntryID {
		t.Fatalf("entry_id=%q, want %q", got.EntryID, entry.EntryID)
	}

	_, ok := lookupCachedEntryByID(dir, entry.EntryID)
	if !ok {
		t.Fatal("expected FindEntryByID to populate lookup cache")
	}
}
