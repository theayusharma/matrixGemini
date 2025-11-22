package modules

import (
	"fmt"
	"math/rand"
	"time"
)

var magicAnswers = []string{
	"It is certain.", "It is decidedly so.", "Without a doubt.", "Yes definitely.",
	"You may rely on it.", "As I see it, yes.", "Most likely.", "Outlook good.",
	"Yes.", "Signs point to yes.", "Reply hazy, try again.", "Ask again later.",
	"Better not tell you now.", "Cannot predict now.", "Concentrate and ask again.",
	"Don't count on it.", "My reply is no.", "My sources say no.",
	"Outlook not so good.", "Very doubtful.",
}

func Magic8Ball(question string) string {
	if question == "" {
		return "Usage: `!8ball <question>`"
	}
	rand.Seed(time.Now().UnixNano())
	answer := magicAnswers[rand.Intn(len(magicAnswers))]
	return fmt.Sprintf("🎱 **Question:** %s\n**Answer:** %s", question, answer)
}

func RussianRoulette(user string) string {
	rand.Seed(time.Now().UnixNano())
	// 1 in 6 chance
	if rand.Intn(6) == 0 {
		return fmt.Sprintf("💥 **BANG!** %s is dead. (F in chat)", user)
	}
	return fmt.Sprintf("😌 **Click.** %s survives... for now.", user)
}
