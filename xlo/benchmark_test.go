package xlo_test

import (
	"strconv"
	"testing"

	"github.com/samber/lo"

	"github.com/v8fg/kit4go/xlo"
)

func BenchmarkFindUniquesInt(b *testing.B) {
	testSet := [][]int{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		{1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2},
		{1, 2, 2, 2, 2, 2, 1, 1, 1, 2, 1, 2},
	}

	for b.Loop() {
		for j := range testSet {
			xlo.Uniq(testSet[j])
		}
	}
}

func BenchmarkFindUniquesStr(b *testing.B) {
	testSet := [][]string{
		{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"},
		{"1", "2", "1", "2", "1", "2", "1", "2", "1", "2", "1", "2"},
		{"1", "2", "2", "2", "2", "2", "1", "1", "1", "2", "1", "2"},
	}

	for b.Loop() {
		for j := range testSet {
			xlo.Uniq(testSet[j])
		}
	}
}

func BenchmarkLoFindUniquesInt(b *testing.B) {
	testSet := [][]int{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		{1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2},
		{1, 2, 2, 2, 2, 2, 1, 1, 1, 2, 1, 2},
	}

	for b.Loop() {
		for j := range testSet {
			xlo.LoUniq(testSet[j])
		}
	}
}

func BenchmarkLoFindUniquesStr(b *testing.B) {
	testSet := [][]string{
		{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"},
		{"1", "2", "1", "2", "1", "2", "1", "2", "1", "2", "1", "2"},
		{"1", "2", "2", "2", "2", "2", "1", "1", "1", "2", "1", "2"},
	}

	for b.Loop() {
		for j := range testSet {
			xlo.LoUniq(testSet[j])
		}
	}
}

func BenchmarkLoMap(b *testing.B) {
	original := lo.Range(1_024)

	for b.Loop() {
		xlo.LoMap(original, func(item int, index int) string {
			return strconv.FormatInt(int64(item), 10)
		})
	}
}

func BenchmarkLopMap(b *testing.B) {
	original := lo.Range(1_024)

	for b.Loop() {
		xlo.LopMap(original, func(item int, index int) string {
			return strconv.FormatInt(int64(item), 10)
		})
	}
}
