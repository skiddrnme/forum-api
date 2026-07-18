package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
)

type PostService struct {
	posts map[int64]forum.Post
	nextID int64
}


func (p *PostService) NewPostService() *PostService{
	return &PostService{
		posts: make(map[int64]forum.Post),
		nextID: 1,
	}
}


func (p *PostService) GetPosts(user_id openapi_types.UUID, thread_id string, limit int, offset int, tag string, authorID string) ([]forum.Post, error){
	var result []forum.Post

	threadIdInt, err := strconv.ParseInt(thread_id, 10, 64)
	if err != nil{
		return []forum.Post{}, fmt.Errorf("неверный thread_id", err)
	}

	for _, post := range p.posts{
		match := true
		if post.ThreadId != threadIdInt{
			match = false
		}
		if post.AuthorId != user_id{
			match = false
		}
		if !match{
			continue
		}
		result = append(result, post)
	}

	sort.Slice(result, func(i, j int) bool{
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}