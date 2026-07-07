package service

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
)

type Thread struct {
	ID      uint
	Title   string
	Author  string
	Content string
}

func NewThreadService(id uint, title, author, content string) *Thread {
	return &Thread{
		ID:      ,
		Title:   title,
		Author:  author,
		Content: content,
	}
}

func (t *Thread) GetThreads() ([]Thread, error) {
	var threads []Thread
	
	file, err := os.Open("./db/threads.json")
	if err != nil{
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&threads)
	if err != nil{
		fmt.Println("Ошибка декодирования", err)
	}

	return threads, nil
}
