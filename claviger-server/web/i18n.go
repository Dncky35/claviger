package web

import (
	"encoding/json"
	"log"
	"strings"
)

// Locales map holds all languages in memory for instant lookups
var locales = make(map[string]map[string]interface{})

// InitI18n reads the embedded JSON files and loads them into memory
func InitI18n() {
	// Add languages you want to support here
	langs := []string{"en", "tr"}

	for _, lang := range langs {
		data, err := TemplatesFS.ReadFile("locales/" + lang + ".json")
		if err != nil {
			log.Printf("⚠️ Warning: Could not load locale %s: %v", lang, err)
			continue
		}

		var dict map[string]interface{}
		if err := json.Unmarshal(data, &dict); err != nil {
			log.Printf("⚠️ Warning: Invalid JSON in locale %s: %v", lang, err)
			continue
		}
		locales[lang] = dict
	}
	log.Println("🌐 i18n Engine Initialized successfully")
}

// Translate handles dot-notation lookups (e.g., "uninstall.title")
func Translate(lang string, key string) string {
	dict, ok := locales[lang]
	if !ok {
		dict = locales["en"] // Fallback to English if language not found
	}

	keys := strings.Split(key, ".")
	var current interface{} = dict

	// Traverse the nested JSON map
	for _, k := range keys {
		if m, isMap := current.(map[string]interface{}); isMap {
			current = m[k]
		} else {
			return key // Return the raw key if not found
		}
	}

	if str, isStr := current.(string); isStr {
		return str
	}
	return key
}

// GetLanguageJSON returns the raw JSON string for the frontend JS
func GetLanguageJSON(lang string) string {
	dict, ok := locales[lang]
	if !ok {
		dict = locales["en"]
	}
	bytes, _ := json.Marshal(dict)
	return string(bytes)
}
