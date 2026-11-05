package store

import "context"

// UpdateRoutine changes a routine's editable fields. Nil pointers leave the
// field untouched; a cron change recomputes next_at from the parsed schedule.
func (s *Store) UpdateRoutine(ctx context.Context, id string, name, prompt, cron *string, enabled *bool, nextAt *int64) (Routine, error) {
	routine, err := s.GetRoutine(ctx, id)
	if err != nil {
		return Routine{}, err
	}
	if name != nil {
		routine.Name = *name
	}
	if prompt != nil {
		routine.Prompt = *prompt
	}
	if cron != nil {
		routine.Cron = *cron
	}
	if enabled != nil {
		routine.Enabled = *enabled
	}
	if nextAt != nil {
		routine.NextAt = *nextAt
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE routines SET name = ?, prompt = ?, cron = ?, enabled = ?, next_at = ? WHERE id = ?`,
		routine.Name, routine.Prompt, routine.Cron, boolInt(routine.Enabled), routine.NextAt, id)
	return routine, err
}
