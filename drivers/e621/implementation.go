package e621

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/WheatleyHDD/libgallery"
	"github.com/WheatleyHDD/libgallery/drivers/internal"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
)

func New() libgallery.Driver {
	client := retryablehttp.NewClient()
	client.Logger = &internal.NoLogger{}
	return &implementation{
		client:  client.StandardClient(),
		limiter: *rate.NewLimiter(2, 1),
	}
}

type implementation struct {
	client  *http.Client
	limiter rate.Limiter
}

func (i *implementation) getJSON(url string, h *http.Client, target interface{}) error {
	i.limiter.Wait(context.Background())
	return internal.GetJSON(url, h, target)
}

func (i *implementation) Search(query string, page uint64, limit uint64) ([]libgallery.Post, int, error) {
	const reqbase = "https://e621.net/posts.json?tags=%s&page=%v&limit=%v"
	url := fmt.Sprintf(reqbase, url.QueryEscape(query), page+1, limit)

	var response struct {
		Posts []post `json:"posts"`
	}

	err := i.getJSON(url, i.client, &response)
	if err != nil {
		if herr, ok := err.(*internal.HTTPError); ok {
			if herr.Code() == http.StatusGone {
				return []libgallery.Post{}, 0, nil
			} else {
				return []libgallery.Post{}, 0, err
			}
		} else {
			return []libgallery.Post{}, 0, err
		}
	}

	var libposts []libgallery.Post

	for _, v := range response.Posts {
		ptime, err := time.Parse(time.RFC3339, v.CreatedAt)
		if err != nil {
			return libposts, 0, err
		}
		libposts = append(libposts, libgallery.Post{
			URL:         fmt.Sprintf("https://e621.net/posts/%v", v.ID),
			ID:          strconv.FormatUint(v.ID, 10),
			Date:        ptime,
			NSFW:        (v.Rating != "s"),
			Description: v.Description,
			Score:       v.Score.Total,
			Tags:        v.Tags.toTagString(),
			Uploader:    strconv.FormatUint(v.UploaderID, 10),
			Source:      v.Source,
		})
	}

	return libposts, 0, err
}

func (i *implementation) File(id string) (libgallery.Files, error) {
	const reqbase = "https://e621.net/posts/%s.json"
	url := fmt.Sprintf(reqbase, id)

	var response struct {
		Post post `json:"post"`
	}
	err := i.getJSON(url, i.client, &response)
	if err != nil {
		return []io.ReadCloser{}, err
	}

	filereader, err := internal.GetReadCloser(response.Post.File.URL, i.client)
	if err != nil {
		return []io.ReadCloser{}, err
	}

	return []io.ReadCloser{filereader}, nil
}

// No API access, will need to implement scraping.
func (i *implementation) Comments(id string) ([]libgallery.Comment, error) {
	return []libgallery.Comment{}, nil
}

func (i *implementation) Name() string {
	return "e621"
}
