// Command devseed fills a LOCAL development database with synthetic clients
// and chats, and creates a development administrator through the API.
//
// Everything it produces is obviously fake: names from a short invented list,
// e-mails on example.com, phones from the +7 900 000-xx-xx block. It refuses
// to run unless DEV_SEED_ALLOWED=1 (set by `make dev`) and never against
// APP_ENV=production.
package main

import (
	"fmt"
	"math/rand"
)

// Client is one synthetic person with a single chat.
type Client struct {
	Email       string
	DisplayName string
	Phone       string
	Messages    []Message
}

// Message is one line of a synthetic chat.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

var firstNames = []string{"Анна", "Борис", "Вера", "Глеб", "Дина", "Егор", "Жанна", "Захар", "Ира", "Кирилл"}
var lastNames = []string{"Тестова", "Примеров", "Образцова", "Демин", "Пробная", "Синтетиков"}

// Synthetic opening lines: everyday topics, no real stories.
var openings = []string{
	"Не могу собраться с мыслями перед важной встречей.",
	"Хочу разобраться, почему откладываю дела на потом.",
	"Как спокойно ответить на резкое письмо коллеги?",
	"Устаю к вечеру и срываюсь на близких, хочу это изменить.",
	"Помогите составить план на неделю, всё валится из рук.",
}

var replies = []string{
	"Давайте начнём с того, что именно вызывает напряжение.",
	"Попробуйте описать ситуацию одним предложением — что происходит?",
	"Хорошо. Что вы уже пробовали и что из этого сработало хотя бы немного?",
}

// Generate returns n synthetic clients; the same seed gives the same data.
func Generate(seed int64, n int) []Client {
	r := rand.New(rand.NewSource(seed))
	out := make([]Client, 0, n)
	for i := 0; i < n; i++ {
		name := firstNames[r.Intn(len(firstNames))] + " " + lastNames[r.Intn(len(lastNames))]
		c := Client{
			Email:       fmt.Sprintf("client%03d@example.com", i+1),
			DisplayName: name,
			Phone:       fmt.Sprintf("+7 900 000-%02d-%02d", (i/100)%100, i%100),
		}
		c.Messages = append(c.Messages,
			Message{Role: "user", Content: openings[r.Intn(len(openings))]},
			Message{Role: "assistant", Content: replies[r.Intn(len(replies))]},
			Message{Role: "user", Content: "Спасибо, попробую. Это тестовый диалог."},
		)
		out = append(out, c)
	}
	return out
}
