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

func TestContainsRepetitivePattern(t *testing.T) {
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
			name:     "short text",
			text:     "hi",
			expected: false,
		},
		{
			name:     "normal text",
			text:     "This is a normal post without repetition",
			expected: false,
		},
		{
			name:     "repeating characters",
			text:     "hellooooooo",
			expected: true,
		},
		{
			name:     "repeating pattern",
			text:     "hello hello hello hello",
			expected: true,
		},
		{
			name:     "repeating pattern with case variation",
			text:     "Hello HELLO hello HeLLo",
			expected: true,
		},
		{
			name:     "repeating emoji",
			text:     "🎉🎉🎉🎉🎉",
			expected: true,
		},
		{
			name:     "repeating two symbols",
			text:     "sksksksksksksksk what is this",
			expected: true,
		},
		{
			name:     "repeating compound unicode runes",
			text:     "👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧 this is lit",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firehose.ContainsRepetitivePattern(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsSpamContent(t *testing.T) {
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
			name:     "normal text",
			text:     "This is a normal post about my day",
			expected: false,
		},
		{
			name:     "onlyfans spam",
			text:     "Check out my OnlyFans.com profile",
			expected: true,
		},
		{
			name:     "follow spam",
			text:     "Follow me! Follow back! F4F",
			expected: true,
		},
		{
			name:     "excessive hashtags",
			text:     "#follow #me #please #right #now #trending #viral",
			expected: true,
		},
		{
			name:     "excessive mentions",
			text:     "@user1 @user2 @user3 @user4 @user5 @user6",
			expected: true,
		},
		{
			name:     "excessive emojis",
			text:     "Hey! 😊😍🥰😘😚😋😛😝😜",
			expected: true,
		},
		{
			name:     "nsfw content",
			text:     "Check out my 18+ content",
			expected: true,
		},
		{
			name:     "repeated hashtags",
			text:     "##trending",
			expected: true,
		},
		{
			name:     "high hashtag ratio",
			text:     "Hi #follow #me #now",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firehose.ContainsSpamContent(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}
