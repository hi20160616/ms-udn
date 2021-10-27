package fetcher

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hi20160616/exhtml"
	"github.com/hi20160616/gears"
	"github.com/hi20160616/ms-udn/configs"
	"github.com/hycka/gocc"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Article struct {
	Id            string
	Title         string
	Content       string
	WebsiteId     string
	WebsiteDomain string
	WebsiteTitle  string
	UpdateTime    *timestamppb.Timestamp
	U             *url.URL
	raw           []byte
	doc           *html.Node
}

var ErrTimeOverDays error = errors.New("article update time out of range")
var ErrIgnoreCate error = errors.New("article is ignored by category")
var ErrIgnoreVIP error = errors.New("article is ignored by vip required")

func NewArticle() *Article {
	return &Article{
		WebsiteDomain: configs.Data.MS["udn"].Domain,
		WebsiteTitle:  configs.Data.MS["udn"].Title,
		WebsiteId:     fmt.Sprintf("%x", md5.Sum([]byte(configs.Data.MS["udn"].Domain))),
	}
}

// List get all articles from database
func (a *Article) List() ([]*Article, error) {
	return load()
}

// Get read database and return the data by rawurl.
func (a *Article) Get(id string) (*Article, error) {
	as, err := load()
	if err != nil {
		return nil, err
	}

	for _, a := range as {
		if a.Id == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("[%s] no article with id: %s",
		configs.Data.MS["udn"].Title, id)
}

func (a *Article) Search(keyword ...string) ([]*Article, error) {
	as, err := load()
	if err != nil {
		return nil, err
	}

	as2 := []*Article{}
	for _, a := range as {
		for _, v := range keyword {
			v = strings.ToLower(strings.TrimSpace(v))
			switch {
			case a.Id == v:
				as2 = append(as2, a)
			case a.WebsiteId == v:
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.Title), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.Content), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.WebsiteDomain), v):
				as2 = append(as2, a)
			case strings.Contains(strings.ToLower(a.WebsiteTitle), v):
				as2 = append(as2, a)
			}
		}
	}
	return as2, nil
}

type ByUpdateTime []*Article

func (u ByUpdateTime) Len() int      { return len(u) }
func (u ByUpdateTime) Swap(i, j int) { u[i], u[j] = u[j], u[i] }
func (u ByUpdateTime) Less(i, j int) bool {
	return u[i].UpdateTime.AsTime().Before(u[j].UpdateTime.AsTime())
}

var timeout = func() time.Duration {
	t, err := time.ParseDuration(configs.Data.MS["udn"].Timeout)
	if err != nil {
		log.Printf("[%s] timeout init error: %v", configs.Data.MS["udn"].Title, err)
		return time.Duration(1 * time.Minute)
	}
	return t
}()

func (a *Article) dail(u string) (*Article, error) {
	var err error
	a.U, err = url.Parse(u)
	if err != nil {
		return nil, err
	}
	a.raw, a.doc, err = exhtml.GetRawAndDoc(a.U, 1*time.Minute)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.WithMessagef(err,
				"404 on url: %s", u)
		}
		if strings.Contains(err.Error(), "invalid header") {
			a.Title = a.U.Path
			a.UpdateTime = timestamppb.Now()
			a.Content, err = a.fmtContent("")
			if err != nil {
				return nil, err
			}
			return a, nil
		} else {
			return nil, err
		}
	}
	return a, nil
}

// fetchArticle fetch article by rawurl
func (a *Article) fetchArticle(rawurl string) (*Article, error) {
	translate := func(x string, err error) (string, error) {
		if err != nil {
			return "", err
		}
		tw2s, err := gocc.New("tw2s")
		if err != nil {
			return "", err
		}
		return tw2s.Convert(x)
	}

	// Dail
	var err error
	a, err = a.dail(rawurl)
	if err != nil {
		return nil, err
	}

	a.Id = fmt.Sprintf("%x", md5.Sum([]byte(rawurl)))

	a.Title, err = a.fetchTitle()
	if err != nil {
		return nil, err
	}

	a.UpdateTime, err = a.fetchUpdateTime()
	if err != nil {
		return nil, err
	}

	// content should be the last step to fetch
	a.Content, err = a.fetchContent()
	if err != nil {
		return nil, err
	}

	a.Content, err = translate(a.fmtContent(a.Content))
	if err != nil {
		return nil, err
	}
	return a, nil

}

func (a *Article) fetchTitle() (string, error) {
	n := exhtml.ElementsByTag(a.doc, "title")
	if n == nil || len(n) == 0 {
		return "", fmt.Errorf("[%s] getTitle error, there is no element <title>",
			configs.Data.MS["udn"].Title)
	}
	title := n[0].FirstChild.Data
	ignores := []string{"| 法律前線", "| 職場觀測", "| 流行消費", "股市",
		"娛樂", "旅遊", "運動", "文教", "數位", " | 聯合晚點評 | 聯合報",
		"| 情慾犯罪", "| 動物星球", "| 星座運勢", "| 紓困振興五倍券",
		"| 稅務法務", "| 地方"}
	for _, v := range ignores {
		if strings.Contains(title, v) {
			return "ignore", ErrIgnoreCate
		}
	}
	rp := strings.NewReplacer(
		" | 全球", "",
		" | 世界萬象", "",
		" | 產經", "",
		" | 雜誌", "",
		" | 生活", "",
		" | 大台北", "",
		" | 地方", "",
		" | 社會", "",
		" | 運動", "",
		" | 娛樂", "",
		" | 健康", "",
		" | 股市", "",
		" | 要聞", "",
		" | 文教", "",
		" | 社論", "",
		" | 評論", "",
		" | 兩岸", "",
		" | 數位", "",
		" | Oops", "",
		" | 網搜追夯事", "",
		" | 奇聞不要看", "",
		" | 雜誌", "",
		" | 旅遊", "",
		" | 聯合新聞網", "",
	)
	title = strings.TrimSpace(rp.Replace(title))
	return gears.ChangeIllegalChar(title), nil
}

func (a *Article) fetchUpdateTime() (*timestamppb.Timestamp, error) {
	if a.raw == nil {
		return nil, errors.Errorf("[%s] fetchUpdateTime: raw is nil: %s",
			configs.Data.MS["udn"].Title, a.U.String())
	}

	t := time.Now() // if no time fetched, return current time
	var err error
	re := regexp.MustCompile(`"datePublished":.*?"(.*?)",`)
	rs := re.FindStringSubmatch(string(a.raw))
	if len(rs) == 2 {
		t, err = time.Parse(time.RFC3339, rs[1])
		if err != nil {
			return nil, errors.WithMessagef(err, "[%s] time pased error: %s",
				configs.Data.MS["udn"].Title, a.U.String())
		}
	}
	if t.Before(time.Now().AddDate(0, 0, -3)) {
		return timestamppb.New(t), ErrTimeOverDays
	}
	return timestamppb.New(t), err
}

func shanghai(t time.Time) time.Time {
	loc := time.FixedZone("UTC", 8*60*60)
	return t.In(loc)
}

func (a *Article) fetchContent() (string, error) {
	if a.doc == nil {
		return "", errors.Errorf("[%s] fetchContent: doc is nil: %s", configs.Data.MS["udn"].Title, a.U.String())
	}
	body := ""
	// fetch redirect url
	sraw := string(a.raw)
	re := regexp.MustCompile(`(?m)<script language=javascript>window\.location\.href="(.*?)";</script>`)
	rs := re.FindStringSubmatch(sraw)
	redirect := ""
	if len(rs) == 2 {
		redirect = rs[1]
		if strings.Contains(redirect, "vip.udn.com") ||
			strings.Contains(redirect, "money.udn.com") ||
			strings.Contains(redirect, "money.udn.com") {
			return "", ErrIgnoreVIP
		}
		// get doc and raw by dail
		a, err := a.dail(redirect)
		if err != nil {
			return body, nil
		}
		if strings.Contains(sraw, "選擇下列方案繼續閱讀：") ||
			strings.Contains(sraw, "訂閱看完整精彩內容") {
			return "", ErrIgnoreVIP
		}
		if strings.Contains(redirect, "opinion.udn.com") {
			bodyN := exhtml.ElementsByTag(a.doc, "main")
			if len(bodyN) == 0 {
				return body, errors.Errorf("no article content matched: %s", a.U.String())
			} else {
				for _, n := range bodyN {
					exhtml.ElementsRmByTag(n, "div")
					exhtml.ElementsRmByTag(n, "h1")
					plist := exhtml.ElementsByTag(n, "p")
					var buf bytes.Buffer
					w := io.Writer(&buf)
					for _, v := range plist {
						if err := html.Render(w, v); err != nil {
							return "", errors.WithMessagef(err,
								"node render to bytes fail: %s", a.U.String())
						}
						repl := strings.NewReplacer("<p>", "", "</p>", "", "「", "“", "」", "”")
						x := repl.Replace(buf.String())
						re := regexp.MustCompile(`(?m)<b.*?>(?P<x>.*?)</b>`)
						x = re.ReplaceAllString(x, "**${x}**")
						re = regexp.MustCompile(`(?m)<strong>(?P<x>.*?)</strong>`)
						x = re.ReplaceAllString(x, "**${x}**")
						re = regexp.MustCompile(`(?m)<a href="(?P<href>.*?)".*?>(?P<x>.*?)</a>`)
						x = re.ReplaceAllString(x, "[${x}](https://opinion.udn.com${href})")
						if strings.TrimSpace(x) != "" {
							body += x + "  \n"
						}
						buf.Reset()
					}

				}
				return body, nil
			}
		}
		if strings.Contains(redirect, "vision.udn.com") {
			bodyN := exhtml.ElementsByTagAndClass(a.doc, "article", "story_article")
			if len(bodyN) == 0 {
				return body, errors.Errorf("no article content matched: %s", a.U.String())
			} else {
				for _, n := range bodyN {
					exhtml.ElementsRmByTag(n, "div")
					exhtml.ElementsRmByTag(n, "h1")
					exhtml.ElementsRmByTag(n, "figure")
					exhtml.ElementsRmByTag(n, "blockquote")
					plist := exhtml.ElementsByTag(n, "h2", "p")
					for _, v := range plist {
						if v.FirstChild != nil {
							x := v.FirstChild.Data
							if v.Data == "h2" {
								body += fmt.Sprintf("\n##%s  \n", x)
							} else if v.Data == "b" {
								body += fmt.Sprintf("**%s**", x)
							} else if strings.TrimSpace(x) != "" {
								body += x + "  \n"
							}
						}
					}

				}
				repl := strings.NewReplacer("「", "“", "」", "”")
				body = repl.Replace(body)
				return body, nil
			}
		}
		if strings.Contains(redirect, "money.udn.com") {
			bodyN := exhtml.ElementsByTagAndClass(a.doc, "section", "article-content__editor")
			if len(bodyN) != 0 {
				var buf bytes.Buffer
				w := io.Writer(&buf)
				for _, n := range bodyN {
					exhtml.ElementsRmByTag(n, "div")
					ps := exhtml.ElementsByTag(n, "p")
					for _, p := range ps {
						if err := html.Render(w, p); err != nil {
							return "", errors.WithMessagef(err, "node render to bytes fail: %s", a.U.String())
						}
						repl := strings.NewReplacer("<p>", "", "</p>", "", "「", "“", "」", "”")
						x := repl.Replace(buf.String())
						if strings.Contains(x, "延伸閱讀：") {
							return body, nil
						}
						re := regexp.MustCompile(`(?m)<b.*?>(?P<x>.*?)</b>`)
						x = re.ReplaceAllString(x, "**${x}**")
						re = regexp.MustCompile(`(?m)<strong>(?P<x>.*?)</strong>`)
						x = re.ReplaceAllString(x, "**${x}**")
						re = regexp.MustCompile(`(?m)<a href="(?P<href>.*?)".*?>(?P<x>.*?)</a>`)
						x = re.ReplaceAllString(x, "[${x}](https://money.udn.com${href})")
						if strings.TrimSpace(x) != "" {
							body += x + "  \n"
						}
						buf.Reset()
					}
				}
				return body, nil
			}
		}
	}
	// fetch main site
	bodyN := exhtml.ElementsByTagAndClass(a.doc, "section", "article-content__editor ")
	if len(bodyN) != 0 {
		var buf bytes.Buffer
		w := io.Writer(&buf)
		for _, n := range bodyN {
			exhtml.ElementsRmByTag(n, "div")
			ps := exhtml.ElementsByTag(n, "p")
			for _, p := range ps {
				if err := html.Render(w, p); err != nil {
					return "", errors.WithMessagef(err, "node render to bytes fail: %s", a.U.String())
				}
				repl := strings.NewReplacer("<p>", "", "</p>", "", "「", "“", "」", "”")
				x := repl.Replace(buf.String())
				if strings.Contains(x, "延伸閱讀：") {
					return body, nil
				}
				re := regexp.MustCompile(`(?m)<b.*?>(?P<x>.*?)</b>`)
				x = re.ReplaceAllString(x, "**${x}**")
				re = regexp.MustCompile(`(?m)<strong>(?P<x>.*?)</strong>`)
				x = re.ReplaceAllString(x, "**${x}**")
				re = regexp.MustCompile(`(?m)<a href="(?P<href>.*?)".*?>(?P<x>.*?)</a>`)
				x = re.ReplaceAllString(x, "[${x}](https://udn.com${href})")
				if strings.TrimSpace(x) != "" {
					body += x + "  \n"
				}
				buf.Reset()
			}
		}
	}
	return body, nil
}

func (a *Article) fmtContent(body string) (string, error) {
	var err error
	title := "# " + a.Title + "\n\n"
	lastupdate := shanghai(a.UpdateTime.AsTime()).Format(time.RFC3339)
	webTitle := fmt.Sprintf(" @ [%s](/list/?v=%[1]s): [%[2]s](http://%[2]s)", a.WebsiteTitle, a.WebsiteDomain)
	u, err := url.QueryUnescape(a.U.String())
	if err != nil {
		u = a.U.String() + "\n\nunescape url error:\n" + err.Error()
	}
	body = title +
		"LastUpdate: " + lastupdate +
		webTitle + "\n\n" +
		"---\n" +
		body + "\n\n" +
		"原地址：" + fmt.Sprintf("[%s](%[1]s)", strings.Split(u, "?tmpl=")[0])
	return body, nil
}
