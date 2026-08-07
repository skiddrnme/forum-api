package service

import (
	"fmt"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
)

type SearchService struct{
	postService *PostService
	threadService *ThreadService
}

var item forum.SearchResultItem

func NewSearch(threadService *ThreadService, postService *PostService) *SearchService{
	return &SearchService{
		threadService: threadService,
		postService: postService,
	}
}


func(s *SearchService) Search(query string, searchType string) ([]forum.SearchResultItem, error){
	var result []forum.SearchResultItem
	highlight := fmt.Sprintf("...%s...", query)
	threadResult := forum.ThreadSearchResult{
		Highlight: &highlight,
	}

	item.FromThreadSearchResult(threadResult)

	return result, nil
}