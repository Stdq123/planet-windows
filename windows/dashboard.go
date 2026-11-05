package main

import "windows/internal/store"

func (l *Library) Dashboard() (store.DashboardSnapshot, error) {
	s, err := l.open()
	if err != nil {
		return store.DashboardSnapshot{}, err
	}
	return s.Dashboard(), nil
}
