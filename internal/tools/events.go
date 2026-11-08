package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// maxEvents caps how many events GetEvents returns. Crash loops produce a
// steady stream of events; the most recent ones carry the diagnosis, and
// older ones only cost tokens.
const maxEvents = 20

// GetEvents returns the namespace's most recent events, oldest first, one per
// line: type, reason, object, message and how many times it repeated.
func (t *Tools) GetEvents(ctx context.Context) (string, error) {
	events, err := t.client.CoreV1().Events(t.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing events in %s: %w", t.namespace, err)
	}
	if len(events.Items) == 0 {
		return fmt.Sprintf("no events in namespace %s\n", t.namespace), nil
	}

	items := events.Items
	// The API returns events in no particular order. Sort oldest to newest. Keep the newest 20
	sort.Slice(items, func(i, j int) bool {
		ti, tj := eventTime(items[i]), eventTime(items[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return items[i].Name < items[j].Name // same second is common; event names are unique, so the order is fixed
	})
	if len(items) > maxEvents {
		items = items[len(items)-maxEvents:]
	}

	var b strings.Builder
	for _, e := range items {
		obj := strings.ToLower(e.InvolvedObject.Kind) + "/" + e.InvolvedObject.Name
		msg := strings.TrimSpace(e.Message)
		fmt.Fprintf(&b, "%s  %s  %s  %s", e.Type, e.Reason, obj, msg)
		if eventCount(e) > 1 {
			b.WriteString("  (repeated)")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// eventTime returns when the event last happened. Older-style events set
// LastTimestamp; newer-style events set EventTime instead.
func eventTime(e corev1.Event) time.Time {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	default:
		return e.CreationTimestamp.Time
	}
}

// eventCount returns how many times the event repeated, from whichever field
// the event style uses.
func eventCount(e corev1.Event) int32 {
	if e.Series != nil && e.Series.Count > e.Count {
		return e.Series.Count
	}
	return e.Count
}
