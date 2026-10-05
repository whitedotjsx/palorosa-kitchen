package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
)

var spanishWeekdays = []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

var spanishMonthNamesFull = []string{
	"", "enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// HumanDate turns "2026-10-02" into "viernes 2 de octubre de 2026".
func HumanDate(iso string) string {
	parsed, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%s %d de %s de %d",
		spanishWeekdays[int(parsed.Weekday())],
		parsed.Day(),
		spanishMonthNamesFull[int(parsed.Month())],
		parsed.Year(),
	)
}

// FormatListText renders a list as WhatsApp text. `when` is an optional label
// such as "HOY" or "MAÑANA".
func FormatListText(list KitchenList, date, when string) string {
	title := labels.Bot.ListTitle
	if when != "" {
		title = strings.ReplaceAll(labels.Bot.ListFor, "{when}", when)
	}
	lines := []string{title}
	if date != "" {
		lines = append(lines, HumanDate(date))
	}

	if len(list.Entries) == 0 {
		lines = append(lines, "", labels.Bot.Empty)
	}
	for _, group := range GroupEntriesByCategory(list) {
		lines = append(lines, "", categoryLabel(group.Category))
		for _, entry := range group.Entries {
			lines = append(lines, entryLine(entry.Quantity, entry.Name, entry.Note))
		}
	}
	if len(list.Unresolved) > 0 {
		lines = append(lines, "", labels.Bot.Unresolved)
		for _, entry := range list.Unresolved {
			lines = append(lines, fmt.Sprintf("- %d de %s (%s)", entry.Quantity, entry.ProductText, reasonLabel(entry.Reason)))
		}
	}
	lines = append(lines, "", labels.Bot.ListHint)
	return strings.Join(lines, "\n")
}

// FormatDiffText renders a change set as WhatsApp text. `when` is an optional
// label such as "HOY" or "MAÑANA".
func FormatDiffText(diff ListDiff, title, note, date, when string) string {
	if title == "" {
		title = labels.Bot.DiffTitle
	}
	lines := []string{title}
	subtitle := HumanDate(date)
	if when != "" {
		subtitle = when + " · " + subtitle
	}
	lines = append(lines, subtitle)
	if note != "" {
		lines = append(lines, strings.ReplaceAll(labels.Bot.Observation, "{text}", note))
	}

	if len(diff.Added) > 0 {
		lines = append(lines, "", labels.Bot.Added)
		for _, entry := range diff.Added {
			lines = append(lines, entryLine(entry.Next, entry.Name, ""))
		}
	}
	if len(diff.Changed) > 0 {
		lines = append(lines, "", labels.Bot.Changed)
		for _, entry := range diff.Changed {
			lines = append(lines, "- "+strings.NewReplacer(
				"{name}", entry.Name,
				"{previous}", fmt.Sprintf("%d", entry.Previous),
				"{next}", fmt.Sprintf("%d", entry.Next),
			).Replace(labels.Bot.ChangedLine))
		}
	}
	if len(diff.Removed) > 0 {
		lines = append(lines, "", labels.Bot.Removed)
		for _, entry := range diff.Removed {
			lines = append(lines, entryLine(entry.Previous, entry.Name, ""))
		}
	}
	if len(lines) == 1 {
		lines = append(lines, labels.Bot.DiffEmpty)
	}
	lines = append(lines, "", labels.Bot.NotificationHint)
	return strings.Join(lines, "\n")
}

// OrderNotice carries everything needed to render an order notification.
type OrderNotice struct {
	Title             string
	When              string
	Date              string
	OrderText         string
	Note              string
	Kind              string // "new" or "update"
	Before            []KitchenListEntry
	After             []KitchenListEntry
	Totals            []KitchenListEntry
	TotalsWerePresent map[string]bool
	FoodChanged       bool
}

// FormatOrderNotice renders a new/updated order notification in plain Spanish.
func FormatOrderNotice(notice OrderNotice) string {
	lines := []string{notice.Title}
	subtitle := HumanDate(notice.Date)
	if notice.When != "" {
		subtitle = notice.When + " · " + subtitle
	}
	lines = append(lines, subtitle)
	if notice.OrderText != "" {
		lines = append(lines, "", strings.ReplaceAll(labels.Bot.OrderPrefix, "{text}", notice.OrderText))
	}
	if notice.Note != "" {
		lines = append(lines, strings.ReplaceAll(labels.Bot.Observation, "{text}", notice.Note))
	}

	if notice.Kind == "new" {
		lines = append(lines, "", labels.Bot.AddToMake)
		lines = append(lines, noticeEntryLines(notice.After, true)...)
		if len(notice.Totals) > 0 {
			lines = append(lines, "", labels.Bot.TotalAfterAdd)
			lines = append(lines, noticeEntryLines(notice.Totals, false)...)
		}
		lines = append(lines, "", labels.Bot.NotificationHint)
		return strings.Join(lines, "\n")
	}

	if !notice.FoodChanged {
		lines = append(lines, "", labels.Bot.NoFoodChange, "", labels.Bot.NotificationHint)
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "", labels.Bot.OrderOriginal)
	lines = append(lines, noticeEntryLines(notice.Before, true)...)

	lines = append(lines, "", labels.Bot.OrderUpdated)
	lines = append(lines, noticeUpdatedLines(notice.Before, notice.After)...)

	if len(notice.Totals) > 0 {
		lines = append(lines, "", labels.Bot.ListAfter)
		lines = append(lines, noticeTotalLines(notice.Totals, notice.TotalsWerePresent)...)
	}
	lines = append(lines, "", labels.Bot.NotificationHint)
	return strings.Join(lines, "\n")
}

func noticeEntryLines(entries []KitchenListEntry, emptyWhenEmpty bool) []string {
	if len(entries) == 0 {
		if emptyWhenEmpty {
			return []string{labels.Bot.EmptySection}
		}
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entryLine(entry.Quantity, entry.Name, entry.Note))
	}
	return out
}

// noticeUpdatedLines lists the order units after the change, quoting removals
// and additions relative to the original order.
func noticeUpdatedLines(before, after []KitchenListEntry) []string {
	beforeQty := map[string]int{}
	for _, entry := range before {
		beforeQty[entry.UnitID] = entry.Quantity
	}
	afterQty := map[string]int{}
	for _, entry := range after {
		afterQty[entry.UnitID] = entry.Quantity
	}
	out := []string{}
	for _, entry := range noticeUnion(before, after) {
		name := entry.Name
		quantity := afterQty[entry.UnitID]
		note := ""
		switch {
		case beforeQty[entry.UnitID] > 0 && quantity == 0:
			count := beforeQty[entry.UnitID]
			template := labels.Bot.RemovedOne
			if count != 1 {
				template = labels.Bot.RemovedMany
			}
			note = strings.ReplaceAll(template, "{n}", fmt.Sprintf("%d", count))
		case beforeQty[entry.UnitID] == 0 && quantity > 0:
			template := labels.Bot.AddedOne
			if quantity != 1 {
				template = labels.Bot.AddedMany
			}
			note = strings.ReplaceAll(template, "{n}", fmt.Sprintf("%d", quantity))
		}
		suffix := ""
		if note != "" {
			suffix = " " + note
		}
		out = append(out, fmt.Sprintf("- %d × %s%s", quantity, name, suffix))
	}
	if len(out) == 0 {
		return []string{labels.Bot.EmptySection}
	}
	return out
}

// noticeTotalLines lists the resulting day totals for the affected units.
func noticeTotalLines(totals []KitchenListEntry, wasPresent map[string]bool) []string {
	out := make([]string, 0, len(totals))
	for _, entry := range totals {
		note := ""
		switch {
		case entry.Quantity == 0:
			note = labels.Bot.NoLongerMake
		case !wasPresent[entry.UnitID]:
			note = labels.Bot.NewInList
		default:
			note = labels.Bot.NewTotalNote
		}
		out = append(out, fmt.Sprintf("- %d × %s %s", entry.Quantity, entry.Name, note))
	}
	return out
}

func noticeUnion(before, after []KitchenListEntry) []KitchenListEntry {
	seen := map[string]bool{}
	out := []KitchenListEntry{}
	for _, entry := range before {
		if !seen[entry.UnitID] {
			out = append(out, entry)
			seen[entry.UnitID] = true
		}
	}
	for _, entry := range after {
		if !seen[entry.UnitID] {
			out = append(out, entry)
			seen[entry.UnitID] = true
		}
	}
	return out
}

func entryLine(quantity int, name, note string) string {
	suffix := ""
	if note != "" {
		suffix = " (" + note + ")"
	}
	return fmt.Sprintf("- %d × %s%s", quantity, name, suffix)
}

func categoryLabel(category string) string {
	if label, ok := labels.CategoryLabels[category]; ok {
		return strings.ToUpper(label)
	}
	return strings.ToUpper(labels.CategoryLabels["other"])
}

func reasonLabel(reason string) string {
	if label, ok := labels.ReasonLabels[reason]; ok {
		return label
	}
	return reason
}
