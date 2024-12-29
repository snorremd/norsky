package firehose_test

import (
	"norsky/firehose"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasEnoughNorwegianLetters(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "only special characters",
			text:     "!@#$%^&*()",
			expected: false,
		},
		{
			name:     "few letters",
			text:     "hi! :) 123456789",
			expected: false,
		},
		{
			name:     "enough regular letters",
			text:     "Dette er en normal norsk tekst",
			expected: true,
		},
		{
			name:     "enough letters with Norwegian characters",
			text:     "Blåbær og røde æbler på trærne",
			expected: true,
		},
		{
			name:     "mixed content with enough letters",
			text:     "Hei! 😊 Dette er en fin dag å være ute! 🌞",
			expected: true,
		},
		{
			name:     "mixed content with too few letters",
			text:     "Hi! 😊 🌞 123 !!! ???",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firehose.HasEnoughNorwegianLetters(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}
