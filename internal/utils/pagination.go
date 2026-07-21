package utils

import forum "stepik.leoscode.http/internal/gen/api"

type PaginationParams struct {
	Limit  int
	Offset int
}

type PaginationResult[T any] struct {
	Items []T
	Meta  forum.PaginationMeta
}

func NewPaginationParams(limit, offset int, defaultLimit int) PaginationParams{
	if limit <= 0{
		limit = defaultLimit
	}

	if limit > 100{
		limit = 100
	}
	
	if offset < 0{
		offset = 0
	}

	return PaginationParams{
		Limit: limit,
		Offset: offset,
	}
}

func ApplyPaginate[T any](items []T, limit, offset int) []T {
    // Проверяем границы
    if len(items) == 0 {
        return []T{}
    }
    
    if offset >= len(items) {
        return []T{}
    }
    
    start := offset
    end := offset + limit
    if end > len(items) {
        end = len(items)
    }
    
    return items[start:end]
}

// ApplyPaginateWithMeta применяет пагинацию с метаданными
func ApplyPaginateWithMeta[T any](items []T, limit, offset int) PaginationResult[T] {
    total := int64(len(items))
    paginated := ApplyPaginate(items, limit, offset)
    
    return PaginationResult[T]{
        Items: paginated,
        Meta: forum.PaginationMeta{
            Limit:  int32(limit),
            Offset: int32(offset),
            Total:  int64(total),
        },
    }
}

// ApplyPaginateWithDefault применяет пагинацию с дефолтными значениями
func ApplyPaginateWithDefault[T any](items []T, limit, offset int) []T {
    // Нормализуем параметры
    if limit <= 0 {
        limit = 20 // дефолтное значение
    }
    if limit > 100 {
        limit = 100
    }
    if offset < 0 {
        offset = 0
    }
    
    return ApplyPaginate(items, limit, offset)
}

// GenericPaginate универсальная пагинация с параметрами
func GenericPaginate[T any](items []T, params PaginationParams) []T {
    return ApplyPaginate(items, params.Limit, params.Offset)
}