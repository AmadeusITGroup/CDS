package cg

import (
	"slices"
	"strings"
)

func Map[T, U any](s []T, f func(T) U) []U {
	var resultList []U
	for _, x := range s {
		resultList = append(resultList, f(x))
	}
	return resultList
}

// Returns a new slice without the element at the index
func SliceWithoutElemAt[T any](s []T, index int) []T {
	ret := make([]T, 0)
	ret = append(ret, s[:index]...)
	return append(ret, s[index+1:]...)
}

func FilterNilFromSlice[T comparable](elems []T) []T {
	isNotNil := func(elem T) bool {
		var zero T
		return elem != zero
	}
	return FilterSlice(elems, isNotNil)
}

// Checks that the slice s of type comparable contains element e of the same type.
//
// Deprecated: use golang's slices.Contains
func Contains[T comparable](s []T, e T) bool {
	for _, val := range s {
		if val == e {
			return true
		}
	}
	return false
}

// Checks if predicate is true for at least one element in the slice
func Any[T any](s []T, predicate func(T) bool) bool {
	for _, x := range s {
		if predicate(x) {
			return true
		}
	}
	return false
}

// returns the first element verifying predicate, return nil equivalent otherwise
func Find[T any](s []T, predicate func(T) bool) T {
	for _, x := range s {
		if predicate(x) {
			return x
		}
	}
	return *new(T)
}

// Merge slices
func Merge[T comparable](slices ...[]T) []T {
	var result []T
	for _, slice := range slices {
		result = append(result, slice...)
	}
	return result
}

// Remove duplicates inside a slice
func Unique[T comparable](slice []T) []T {
	var result []T

	visitedItems := make(map[T]struct{})

	for _, item := range slice {
		if _, exists := visitedItems[item]; !exists {
			visitedItems[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func RemoveElemFromSlice[T comparable](s []T, value T) []T {
	index := slices.Index(s, value)
	if index == -1 {
		return s
	}
	return append(s[:index], s[index+1:]...)
}

func FindElemFromSlice[T any](s []T, predicate func(T) bool) (T, bool) {
	for _, elem := range s {
		if predicate(elem) {
			return elem, true
		}
	}
	var zero T // Zero value for type T
	return zero, false
}

func AddElementToSliceIfNotExists[T comparable](s []T, value T) []T {
	index := slices.Index(s, value)
	if index != -1 {
		return s
	}
	return append(s, value)
}

func FilterSlice[T any](s []T, predicate func(T) bool) []T {
	var filteredList []T
	for _, elem := range s {
		if predicate(elem) {
			filteredList = append(filteredList, elem)
		}
	}
	return filteredList
}

func VariadicJoin(sep string, parts ...string) string {
	return strings.Join(parts, sep)
}

// Used to get the first parent directory of a given path
// e.g. /home/user/parent/child -> home
// e.g. .devcontainer/devcontainer.json -> .devcontainer
func GetFirstParentDir(path string) string {
	if path == "" {
		return ""
	}
	files := strings.Split(path, "/")
	if files[0] == "" && len(files) > 1 {
		return files[1]
	}
	return files[0]
}
