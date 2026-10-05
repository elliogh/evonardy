package study

import (
	"fmt"
	"os"
	"path/filepath"

	"evonardy/internal/library"
	"evonardy/internal/storage"
)

// Install copies validated immutable selected packages only. The destination
// must be exclusively owned; existing models and their metadata are preserved.
func Install(source, destination *storage.Store, s State) error {
	if s.Status != "completed" {
		return fmt.Errorf("only a completed study can install results")
	}
	if s.SelectedID != s.IncumbentID && (s.Verdict == nil || s.Verdict.Status != "confirmed" || !s.SelectionLocked || s.SelectedID != s.CandidateID) {
		return fmt.Errorf("replacement has no confirmation")
	}
	ids := []string{s.SelectedID}
	if s.BestNewID != "" && s.BestNewID != s.SelectedID {
		ids = append(ids, s.BestNewID)
	}
	for _, id := range ids {
		if id == library.HeuristicID || id == library.RandomID {
			continue
		}
		if _, err := library.New(source).Freeze(id); err != nil {
			return err
		}
		files := map[string][]byte{}
		for _, name := range []string{"manifest.json", "model.json", "metadata.json"} {
			data, err := source.Read(filepath.Join("bots", id, name), 1<<20)
			if err != nil {
				return err
			}
			files[name] = data
		}
		publishErr := destination.PublishNew(filepath.Join("bots", id), files)
		if publishErr != nil && !os.IsExist(publishErr) {
			return publishErr
		}
		if publishErr == nil && id != s.IncumbentID {
			bots := library.New(destination)
			card, err := bots.Get(id)
			if err != nil {
				return err
			}
			label := "Candidate"
			if id == s.SelectedID {
				label = "Confirmed"
			}
			// Names are mutable metadata; published inference bytes stay intact.
			name := card.Name
			if len(name) > 100 {
				name = id[:16]
			}
			_, err = bots.Rename(id, library.RenameRequest{Command: library.Command{CommandID: digest([]byte(s.ID + "/install/" + id + "/" + label))[:32], ExpectedVersion: card.MetadataVersion}, Name: label + " — " + name})
			if err != nil {
				return err
			}
		}
		if _, err := library.New(destination).Freeze(id); err != nil {
			return err
		}
	}
	return nil
}
