package fetcher

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/hi20160616/exhtml"
	"github.com/pkg/errors"
)

func TestFetchTitle(t *testing.T) {
	tests := []struct {
		url   string
		title string
	}{
		// {"https://udn.com/news/story/6811/5841452", "本想建設火車！一挖竟出土“超巨量瑪雅文明古物”"},
		// {"https://udn.com/news/story/6812/4532169", "金正恩性愛列車揭秘“歡樂組”專挑高妹處女 最小未滿13歲"},
		{"https://udn.com/news/story/12177/5843524", "願景／綠電浪潮 公民電廠兩大難題 | 綠能缺口怎麼補"},
	}
	for _, tc := range tests {
		a := NewArticle()
		u, err := url.Parse(tc.url)
		if err != nil {
			t.Error(err)
		}
		a.U = u
		// Dail
		a.raw, a.doc, err = exhtml.GetRawAndDoc(a.U, timeout)
		if err != nil {
			t.Error(err)
		}
		got, err := a.fetchTitle()
		if err != nil {
			if !errors.Is(err, ErrTimeOverDays) {
				t.Error(err)
			} else {
				fmt.Println("ignore pass test: ", tc.url)
			}
		} else {
			if tc.title != got {
				t.Errorf("\nwant: %s\n got: %s", tc.title, got)
			}
		}
	}

}

func TestFetchUpdateTime(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// {"https://udn.com/news/story/6811/5841452", "2021-10-25 12:36:08 +0800 UTC"},
		// {"https://udn.com/news/story/6812/4532169", "2020-05-01 09:20:37 +0800 UTC"},
		{"https://udn.com/news/story/12177/5843524", "2021-10-26 11:05:00 +0800 UTC"},
	}
	var err error
	for _, tc := range tests {
		a := NewArticle()
		a.U, err = url.Parse(tc.url)
		if err != nil {
			t.Error(err)
		}
		// Dail
		a.raw, a.doc, err = exhtml.GetRawAndDoc(a.U, timeout)
		if err != nil {
			t.Error(err)
		}
		tt, err := a.fetchUpdateTime()
		if err != nil {
			if !errors.Is(err, ErrTimeOverDays) {
				t.Error(err)
			}
		}
		ttt := tt.AsTime()
		got := shanghai(ttt)
		if got.String() != tc.want {
			t.Errorf("\nwant: %s\n got: %s", tc.want, got.String())
		}
	}
}

func TestFetchContent(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://udn.com/news/story/6811/5841452", "本想建設火車！一挖竟出土「超巨量瑪雅文明古物」"},
		{"https://udn.com/news/story/6812/4532169", "金正恩性愛列車揭秘「歡樂組」專挑高妹處女 最小未滿13歲"},
		{"https://udn.com/news/story/12177/5843524", "金正恩性愛列車揭秘「歡樂組」專挑高妹處女 最小未滿13歲"},
		{"https://udn.com/news/story/120974/5843356", "金正恩性愛列車揭秘「歡樂組」專挑高妹處女 最小未滿13歲"},
	}
	var err error

	for _, tc := range tests {
		a := NewArticle()
		a.U, err = url.Parse(tc.url)
		if err != nil {
			t.Error(err)
		}
		// Dail
		a.raw, a.doc, err = exhtml.GetRawAndDoc(a.U, timeout)
		if err != nil {
			t.Error(err)
		}
		c, err := a.fetchContent()
		if err != nil {
			t.Error(err)
		}
		fmt.Println(c)
	}
}

func TestFetchArticle(t *testing.T) {
	tests := []struct {
		url string
		err error
	}{
		{"https://udn.com/news/story/6811/5841452", ErrTimeOverDays},
		// {"https://udn.com/news/story/6812/4532169", nil},
		// {"https://udn.com/news/story/6897/5843460", nil},
		// {"https://udn.com/news/story/6656/5843558", nil},
		// {"https://udn.com/news/story/12177/5843524", nil},
		// {"https://udn.com/news/story/120974/5843356", nil},
		// {"https://udn.com/news/story/7314/5844797", nil},
		{"https://udn.com/news/story/7320/5844909", nil},
	}
	for _, tc := range tests {
		a := NewArticle()
		a, err := a.fetchArticle(tc.url)
		if err != nil {
			if !errors.Is(err, ErrTimeOverDays) {
				t.Error(err)
			} else {
				fmt.Println("ignore old news pass test: ", tc.url)
			}
		} else {
			fmt.Println("pass test: ", a.Content)
		}
	}
}
