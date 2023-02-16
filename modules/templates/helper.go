// Copyright 2018 The Gitea Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package templates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"mime"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	texttmpl "text/template"
	"time"
	"unicode"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/avatars"
	issues_model "code.gitea.io/gitea/models/issues"
	"code.gitea.io/gitea/models/organization"
	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/emoji"
	"code.gitea.io/gitea/modules/git"
	giturl "code.gitea.io/gitea/modules/git/url"
	gitea_html "code.gitea.io/gitea/modules/html"
	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/markdown"
	"code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/svg"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/gitdiff"

	"github.com/editorconfig/editorconfig-core-go/v2"
)

// Used from static.go && dynamic.go
var mailSubjectSplit = regexp.MustCompile(`(?m)^-{3,}[\s]*$`)

// NewFuncMap returns functions for injecting to templates
func NewFuncMap() []template.FuncMap {
	return []template.FuncMap{map[string]interface{}{
		"GoVer": func() string {
			return util.ToTitleCase(runtime.Version())
		},
		"UseHTTPS": func() bool {
			return strings.HasPrefix(setting.AppURL, "https")
		},
		"AppName": func() string {
			return setting.AppName
		},
		"AppSubUrl": func() string {
			return setting.AppSubURL
		},
		"AssetUrlPrefix": func() string {
			return setting.StaticURLPrefix + "/assets"
		},
		"AppUrl": func() string {
			// The usage of AppUrl should be avoided as much as possible,
			// because the AppURL(ROOT_URL) may not match user's visiting site and the ROOT_URL in app.ini may be incorrect.
			// And it's difficult for Gitea to guess absolute URL correctly with zero configuration,
			// because Gitea doesn't know whether the scheme is HTTP or HTTPS unless the reverse proxy could tell Gitea.
			return setting.AppURL
		},
		"AppVer": func() string {
			return setting.AppVer
		},
		"AppBuiltWith": func() string {
			return setting.AppBuiltWith
		},
		"AppDomain": func() string {
			return setting.Domain
		},
		"AssetVersion": func() string {
			return setting.AssetVersion
		},
		"DisableGravatar": func(ctx context.Context) bool {
			return system_model.GetSettingBool(ctx, system_model.KeyPictureDisableGravatar)
		},
		"DefaultShowFullName": func() bool {
			return setting.UI.DefaultShowFullName
		},
		"ShowFooterTemplateLoadTime": func() bool {
			return setting.ShowFooterTemplateLoadTime
		},
		"LoadTimes": func(startTime time.Time) string {
			return fmt.Sprint(time.Since(startTime).Nanoseconds()/1e6) + "ms"
		},
		"AllowedReactions": func() []string {
			return setting.UI.Reactions
		},
		"CustomEmojis": func() map[string]string {
			return setting.UI.CustomEmojisMap
		},
		"Safe":           Safe,
		"SafeJS":         SafeJS,
		"JSEscape":       JSEscape,
		"Str2html":       Str2html,
		"TimeSince":      timeutil.TimeSince,
		"TimeSinceUnix":  timeutil.TimeSinceUnix,
		"FileSize":       base.FileSize,
		"PrettyNumber":   base.PrettyNumber,
		"JsPrettyNumber": JsPrettyNumber,
		"Subtract":       base.Subtract,
		"EntryIcon":      base.EntryIcon,
		"MigrationIcon":  MigrationIcon,
		"Add": func(a ...int) int {
			sum := 0
			for _, val := range a {
				sum += val
			}
			return sum
		},
		"Mul": func(a ...int) int {
			sum := 1
			for _, val := range a {
				sum *= val
			}
			return sum
		},
		"ActionIcon": ActionIcon,
		"DateFmtLong": func(t time.Time) string {
			return t.Format(time.RFC1123Z)
		},
		"DateFmtShort": func(t time.Time) string {
			return t.Format("Jan 02, 2006")
		},
		"CountFmt": base.FormatNumberSI,
		"SubStr": func(str string, start, length int) string {
			if len(str) == 0 {
				return ""
			}
			end := start + length
			if length == -1 {
				end = len(str)
			}
			if len(str) < end {
				return str
			}
			return str[start:end]
		},
		"EllipsisString":                 base.EllipsisString,
		"DiffTypeToStr":                  DiffTypeToStr,
		"DiffLineTypeToStr":              DiffLineTypeToStr,
		"ShortSha":                       base.ShortSha,
		"ActionContent2Commits":          ActionContent2Commits,
		"PathEscape":                     url.PathEscape,
		"PathEscapeSegments":             util.PathEscapeSegments,
		"URLJoin":                        util.URLJoin,
		"RenderCommitMessage":            RenderCommitMessage,
		"RenderCommitMessageLink":        RenderCommitMessageLink,
		"RenderCommitMessageLinkSubject": RenderCommitMessageLinkSubject,
		"RenderCommitBody":               RenderCommitBody,
		"RenderCodeBlock":                RenderCodeBlock,
		"RenderIssueTitle":               RenderIssueTitle,
		"RenderEmoji":                    RenderEmoji,
		"RenderEmojiPlain":               emoji.ReplaceAliases,
		"ReactionToEmoji":                ReactionToEmoji,
		"RenderNote":                     RenderNote,
		"RenderMarkdownToHtml": func(input string) template.HTML {
			output, err := markdown.RenderString(&markup.RenderContext{
				URLPrefix: setting.AppSubURL,
			}, input)
			if err != nil {
				log.Error("RenderString: %v", err)
			}
			return template.HTML(output)
		},
		"IsMultilineCommitMessage": IsMultilineCommitMessage,
		"ThemeColorMetaTag": func() string {
			return setting.UI.ThemeColorMetaTag
		},
		"MetaAuthor": func() string {
			return setting.UI.Meta.Author
		},
		"MetaDescription": func() string {
			return setting.UI.Meta.Description
		},
		"MetaKeywords": func() string {
			return setting.UI.Meta.Keywords
		},
		"UseServiceWorker": func() bool {
			return setting.UI.UseServiceWorker
		},
		"EnableTimetracking": func() bool {
			return setting.Service.EnableTimetracking
		},
		"FilenameIsImage": func(filename string) bool {
			mimeType := mime.TypeByExtension(filepath.Ext(filename))
			return strings.HasPrefix(mimeType, "image/")
		},
		"TabSizeClass": func(ec interface{}, filename string) string {
			var (
				value *editorconfig.Editorconfig
				ok    bool
			)
			if ec != nil {
				if value, ok = ec.(*editorconfig.Editorconfig); !ok || value == nil {
					return "tab-size-8"
				}
				def, err := value.GetDefinitionForFilename(filename)
				if err != nil {
					log.Error("tab size class: getting definition for filename: %v", err)
					return "tab-size-8"
				}
				if def.TabWidth > 0 {
					return fmt.Sprintf("tab-size-%d", def.TabWidth)
				}
			}
			return "tab-size-8"
		},
		"SubJumpablePath": func(str string) []string {
			var path []string
			index := strings.LastIndex(str, "/")
			if index != -1 && index != len(str) {
				path = append(path, str[0:index+1], str[index+1:])
			} else {
				path = append(path, str)
			}
			return path
		},
		"DiffStatsWidth": func(adds, dels int) string {
			return fmt.Sprintf("%f", float64(adds)/(float64(adds)+float64(dels))*100)
		},
		"Json": func(in interface{}) string {
			out, err := json.Marshal(in)
			if err != nil {
				return ""
			}
			return string(out)
		},
		"JsonPrettyPrint": func(in string) string {
			var out bytes.Buffer
			err := json.Indent(&out, []byte(in), "", "  ")
			if err != nil {
				return ""
			}
			return out.String()
		},
		"DisableGitHooks": func() bool {
			return setting.DisableGitHooks
		},
		"DisableWebhooks": func() bool {
			return setting.DisableWebhooks
		},
		"DisableImportLocal": func() bool {
			return !setting.ImportLocalPaths
		},
		"Dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"Printf":   fmt.Sprintf,
		"Escape":   Escape,
		"Sec2Time": util.SecToTime,
		"ParseDeadline": func(deadline string) []string {
			return strings.Split(deadline, "|")
		},
		"DefaultTheme": func() string {
			return setting.UI.DefaultTheme
		},
		// pass key-value pairs to a partial template which receives them as a dict
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values) == 0 {
				return nil, errors.New("invalid dict call")
			}

			dict := make(map[string]interface{})
			return util.MergeInto(dict, values...)
		},
		/* like dict but merge key-value pairs into the first dict and return it */
		"mergeinto": func(root map[string]interface{}, values ...interface{}) (map[string]interface{}, error) {
			if len(values) == 0 {
				return nil, errors.New("invalid mergeinto call")
			}

			dict := make(map[string]interface{})
			for key, value := range root {
				dict[key] = value
			}

			return util.MergeInto(dict, values...)
		},
		"percentage": func(n int, values ...int) float32 {
			sum := 0
			for i := 0; i < len(values); i++ {
				sum += values[i]
			}
			return float32(n) * 100 / float32(sum)
		},
		"CommentMustAsDiff":   gitdiff.CommentMustAsDiff,
		"MirrorRemoteAddress": mirrorRemoteAddress,
		"NotificationSettings": func() map[string]interface{} {
			return map[string]interface{}{
				"MinTimeout":            int(setting.UI.Notification.MinTimeout / time.Millisecond),
				"TimeoutStep":           int(setting.UI.Notification.TimeoutStep / time.Millisecond),
				"MaxTimeout":            int(setting.UI.Notification.MaxTimeout / time.Millisecond),
				"EventSourceUpdateTime": int(setting.UI.Notification.EventSourceUpdateTime / time.Millisecond),
			}
		},
		"containGeneric": func(arr, v interface{}) bool {
			arrV := reflect.ValueOf(arr)
			if arrV.Kind() == reflect.String && reflect.ValueOf(v).Kind() == reflect.String {
				return strings.Contains(arr.(string), v.(string))
			}

			if arrV.Kind() == reflect.Slice {
				for i := 0; i < arrV.Len(); i++ {
					iV := arrV.Index(i)
					if !iV.CanInterface() {
						continue
					}
					if iV.Interface() == v {
						return true
					}
				}
			}

			return false
		},
		"contain": func(s []int64, id int64) bool {
			for i := 0; i < len(s); i++ {
				if s[i] == id {
					return true
				}
			}
			return false
		},
		"svg":            svg.RenderHTML,
		"avatar":         Avatar,
		"avatarHTML":     AvatarHTML,
		"avatarByAction": AvatarByAction,
		"avatarByEmail":  AvatarByEmail,
		"repoAvatar":     RepoAvatar,
		"SortArrow": func(normSort, revSort, urlSort string, isDefault bool) template.HTML {
			// if needed
			if len(normSort) == 0 || len(urlSort) == 0 {
				return ""
			}

			if len(urlSort) == 0 && isDefault {
				// if sort is sorted as default add arrow tho this table header
				if isDefault {
					return svg.RenderHTML("octicon-triangle-down", 16)
				}
			} else {
				// if sort arg is in url test if it correlates with column header sort arguments
				// the direction of the arrow should indicate the "current sort order", up means ASC(normal), down means DESC(rev)
				if urlSort == normSort {
					// the table is sorted with this header normal
					return svg.RenderHTML("octicon-triangle-up", 16)
				} else if urlSort == revSort {
					// the table is sorted with this header reverse
					return svg.RenderHTML("octicon-triangle-down", 16)
				}
			}
			// the table is NOT sorted with this header
			return ""
		},
		"RenderLabels": func(labels []*issues_model.Label, repoLink string) template.HTML {
			htmlCode := `<span class="labels-list">`
			for _, label := range labels {
				// Protect against nil value in labels - shouldn't happen but would cause a panic if so
				if label == nil {
					continue
				}
				htmlCode += fmt.Sprintf("<a href='%s/issues?labels=%d' class='ui label' style='color: %s !important; background-color: %s !important' title='%s'>%s</a> ",
					repoLink, label.ID, label.ForegroundColor(), label.Color, html.EscapeString(label.Description), RenderEmoji(label.Name))
			}
			htmlCode += "</span>"
			return template.HTML(htmlCode)
		},
		"MermaidMaxSourceCharacters": func() int {
			return setting.MermaidMaxSourceCharacters
		},
		"Join":        strings.Join,
		"QueryEscape": url.QueryEscape,
		"DotEscape":   DotEscape,
		"Iterate": func(arg interface{}) (items []uint64) {
			count := uint64(0)
			switch val := arg.(type) {
			case uint64:
				count = val
			case *uint64:
				count = *val
			case int64:
				if val < 0 {
					val = 0
				}
				count = uint64(val)
			case *int64:
				if *val < 0 {
					*val = 0
				}
				count = uint64(*val)
			case int:
				if val < 0 {
					val = 0
				}
				count = uint64(val)
			case *int:
				if *val < 0 {
					*val = 0
				}
				count = uint64(*val)
			case uint:
				count = uint64(val)
			case *uint:
				count = uint64(*val)
			case int32:
				if val < 0 {
					val = 0
				}
				count = uint64(val)
			case *int32:
				if *val < 0 {
					*val = 0
				}
				count = uint64(*val)
			case uint32:
				count = uint64(val)
			case *uint32:
				count = uint64(*val)
			case string:
				cnt, _ := strconv.ParseInt(val, 10, 64)
				if cnt < 0 {
					cnt = 0
				}
				count = uint64(cnt)
			}
			if count <= 0 {
				return items
			}
			for i := uint64(0); i < count; i++ {
				items = append(items, i)
			}
			return items
		},
		"HasPrefix": strings.HasPrefix,
		"CompareLink": func(baseRepo, repo *repo_model.Repository, branchName string) string {
			var curBranch string
			if repo.ID != baseRepo.ID {
				curBranch += fmt.Sprintf("%s/%s:", url.PathEscape(repo.OwnerName), url.PathEscape(repo.Name))
			}
			curBranch += util.PathEscapeSegments(branchName)

			return fmt.Sprintf("%s/compare/%s...%s",
				baseRepo.Link(),
				util.PathEscapeSegments(baseRepo.DefaultBranch),
				curBranch,
			)
		},
		"RefShortName": func(ref string) string {
			return git.RefName(ref).ShortName()
		},
	}}
}

// NewTextFuncMap returns functions for injecting to text templates
// It's a subset of those used for HTML and other templates
func NewTextFuncMap() []texttmpl.FuncMap {
	return []texttmpl.FuncMap{map[string]interface{}{
		"GoVer": func() string {
			return util.ToTitleCase(runtime.Version())
		},
		"AppName": func() string {
			return setting.AppName
		},
		"AppSubUrl": func() string {
			return setting.AppSubURL
		},
		"AppUrl": func() string {
			return setting.AppURL
		},
		"AppVer": func() string {
			return setting.AppVer
		},
		"AppBuiltWith": func() string {
			return setting.AppBuiltWith
		},
		"AppDomain": func() string {
			return setting.Domain
		},
		"TimeSince":     timeutil.TimeSince,
		"TimeSinceUnix": timeutil.TimeSinceUnix,
		"DateFmtLong": func(t time.Time) string {
			return t.Format(time.RFC1123Z)
		},
		"DateFmtShort": func(t time.Time) string {
			return t.Format("Jan 02, 2006")
		},
		"SubStr": func(str string, start, length int) string {
			if len(str) == 0 {
				return ""
			}
			end := start + length
			if length == -1 {
				end = len(str)
			}
			if len(str) < end {
				return str
			}
			return str[start:end]
		},
		"EllipsisString": base.EllipsisString,
		"URLJoin":        util.URLJoin,
		"Dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"Printf":   fmt.Sprintf,
		"Escape":   Escape,
		"Sec2Time": util.SecToTime,
		"ParseDeadline": func(deadline string) []string {
			return strings.Split(deadline, "|")
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values) == 0 {
				return nil, errors.New("invalid dict call")
			}

			dict := make(map[string]interface{})

			for i := 0; i < len(values); i++ {
				switch key := values[i].(type) {
				case string:
					i++
					if i == len(values) {
						return nil, errors.New("specify the key for non array values")
					}
					dict[key] = values[i]
				case map[string]interface{}:
					m := values[i].(map[string]interface{})
					for i, v := range m {
						dict[i] = v
					}
				default:
					return nil, errors.New("dict values must be maps")
				}
			}
			return dict, nil
		},
		"percentage": func(n int, values ...int) float32 {
			sum := 0
			for i := 0; i < len(values); i++ {
				sum += values[i]
			}
			return float32(n) * 100 / float32(sum)
		},
		"Add": func(a ...int) int {
			sum := 0
			for _, val := range a {
				sum += val
			}
			return sum
		},
		"Mul": func(a ...int) int {
			sum := 1
			for _, val := range a {
				sum *= val
			}
			return sum
		},
		"QueryEscape": url.QueryEscape,
	}}
}

// AvatarHTML creates the HTML for an avatar
func AvatarHTML(src string, size int, class, name string) template.HTML {
	sizeStr := fmt.Sprintf(`%d`, size)

	if name == "" {
		name = "avatar"
	}

	return template.HTML(`<img class="` + class + `" src="` + src + `" title="` + html.EscapeString(name) + `" width="` + sizeStr + `" height="` + sizeStr + `"/>`)
}

// Avatar renders user avatars. args: user, size (int), class (string)
func Avatar(ctx context.Context, item interface{}, others ...interface{}) template.HTML {
	size, class := gitea_html.ParseSizeAndClass(avatars.DefaultAvatarPixelSize, avatars.DefaultAvatarClass, others...)

	switch t := item.(type) {
	case *user_model.User:
		src := t.AvatarLinkWithSize(ctx, size*setting.Avatar.RenderedSizeFactor)
		if src != "" {
			return AvatarHTML(src, size, class, t.DisplayName())
		}
	case *repo_model.Collaborator:
		src := t.AvatarLinkWithSize(ctx, size*setting.Avatar.RenderedSizeFactor)
		if src != "" {
			return AvatarHTML(src, size, class, t.DisplayName())
		}
	case *organization.Organization:
		src := t.AsUser().AvatarLinkWithSize(ctx, size*setting.Avatar.RenderedSizeFactor)
		if src != "" {
			return AvatarHTML(src, size, class, t.AsUser().DisplayName())
		}
	}

	return template.HTML("")
}

// AvatarByAction renders user avatars from action. args: action, size (int), class (string)
func AvatarByAction(ctx context.Context, action *activities_model.Action, others ...interface{}) template.HTML {
	action.LoadActUser(ctx)
	return Avatar(ctx, action.ActUser, others...)
}

// RepoAvatar renders repo avatars. args: repo, size(int), class (string)
func RepoAvatar(repo *repo_model.Repository, others ...interface{}) template.HTML {
	size, class := gitea_html.ParseSizeAndClass(avatars.DefaultAvatarPixelSize, avatars.DefaultAvatarClass, others...)

	src := repo.RelAvatarLink()
	if src != "" {
		return AvatarHTML(src, size, class, repo.FullName())
	}
	return template.HTML("")
}

// AvatarByEmail renders avatars by email address. args: email, name, size (int), class (string)
func AvatarByEmail(ctx context.Context, email, name string, others ...interface{}) template.HTML {
	size, class := gitea_html.ParseSizeAndClass(avatars.DefaultAvatarPixelSize, avatars.DefaultAvatarClass, others...)
	src := avatars.GenerateEmailAvatarFastLink(ctx, email, size*setting.Avatar.RenderedSizeFactor)

	if src != "" {
		return AvatarHTML(src, size, class, name)
	}

	return template.HTML("")
}

// Safe render raw as HTML
func Safe(raw string) template.HTML {
	return template.HTML(raw)
}

// SafeJS renders raw as JS
func SafeJS(raw string) template.JS {
	return template.JS(raw)
}

// Str2html render Markdown text to HTML
func Str2html(raw string) template.HTML {
	return template.HTML(markup.Sanitize(raw))
}

// Escape escapes a HTML string
func Escape(raw string) string {
	return html.EscapeString(raw)
}

// JSEscape escapes a JS string
func JSEscape(raw string) string {
	return template.JSEscapeString(raw)
}

// DotEscape wraps a dots in names with ZWJ [U+200D] in order to prevent autolinkers from detecting these as urls
func DotEscape(raw string) string {
	return strings.ReplaceAll(raw, ".", "\u200d.\u200d")
}

// RenderCommitMessage renders commit message with XSS-safe and special links.
func RenderCommitMessage(ctx context.Context, msg, urlPrefix string, metas map[string]string) template.HTML {
	return RenderCommitMessageLink(ctx, msg, urlPrefix, "", metas)
}

// RenderCommitMessageLink renders commit message as a XXS-safe link to the provided
// default url, handling for special links.
func RenderCommitMessageLink(ctx context.Context, msg, urlPrefix, urlDefault string, metas map[string]string) template.HTML {
	cleanMsg := template.HTMLEscapeString(msg)
	// we can safely assume that it will not return any error, since there
	// shouldn't be any special HTML.
	fullMessage, err := markup.RenderCommitMessage(&markup.RenderContext{
		Ctx:         ctx,
		URLPrefix:   urlPrefix,
		DefaultLink: urlDefault,
		Metas:       metas,
	}, cleanMsg)
	if err != nil {
		log.Error("RenderCommitMessage: %v", err)
		return ""
	}
	msgLines := strings.Split(strings.TrimSpace(fullMessage), "\n")
	if len(msgLines) == 0 {
		return template.HTML("")
	}
	return template.HTML(msgLines[0])
}

// RenderCommitMessageLinkSubject renders commit message as a XXS-safe link to
// the provided default url, handling for special links without email to links.
func RenderCommitMessageLinkSubject(ctx context.Context, msg, urlPrefix, urlDefault string, metas map[string]string) template.HTML {
	msgLine := strings.TrimLeftFunc(msg, unicode.IsSpace)
	lineEnd := strings.IndexByte(msgLine, '\n')
	if lineEnd > 0 {
		msgLine = msgLine[:lineEnd]
	}
	msgLine = strings.TrimRightFunc(msgLine, unicode.IsSpace)
	if len(msgLine) == 0 {
		return template.HTML("")
	}

	// we can safely assume that it will not return any error, since there
	// shouldn't be any special HTML.
	renderedMessage, err := markup.RenderCommitMessageSubject(&markup.RenderContext{
		Ctx:         ctx,
		URLPrefix:   urlPrefix,
		DefaultLink: urlDefault,
		Metas:       metas,
	}, template.HTMLEscapeString(msgLine))
	if err != nil {
		log.Error("RenderCommitMessageSubject: %v", err)
		return template.HTML("")
	}
	return template.HTML(renderedMessage)
}

// RenderCommitBody extracts the body of a commit message without its title.
func RenderCommitBody(ctx context.Context, msg, urlPrefix string, metas map[string]string) template.HTML {
	msgLine := strings.TrimRightFunc(msg, unicode.IsSpace)
	lineEnd := strings.IndexByte(msgLine, '\n')
	if lineEnd > 0 {
		msgLine = msgLine[lineEnd+1:]
	} else {
		return template.HTML("")
	}
	msgLine = strings.TrimLeftFunc(msgLine, unicode.IsSpace)
	if len(msgLine) == 0 {
		return template.HTML("")
	}

	renderedMessage, err := markup.RenderCommitMessage(&markup.RenderContext{
		Ctx:       ctx,
		URLPrefix: urlPrefix,
		Metas:     metas,
	}, template.HTMLEscapeString(msgLine))
	if err != nil {
		log.Error("RenderCommitMessage: %v", err)
		return ""
	}
	return template.HTML(renderedMessage)
}

// Match text that is between back ticks.
var codeMatcher = regexp.MustCompile("`([^`]+)`")

// RenderCodeBlock renders "`…`" as highlighted "<code>" block.
// Intended for issue and PR titles, these containers should have styles for "<code>" elements
func RenderCodeBlock(htmlEscapedTextToRender template.HTML) template.HTML {
	htmlWithCodeTags := codeMatcher.ReplaceAllString(string(htmlEscapedTextToRender), "<code>$1</code>") // replace with HTML <code> tags
	return template.HTML(htmlWithCodeTags)
}

// RenderIssueTitle renders issue/pull title with defined post processors
func RenderIssueTitle(ctx context.Context, text, urlPrefix string, metas map[string]string) template.HTML {
	renderedText, err := markup.RenderIssueTitle(&markup.RenderContext{
		Ctx:       ctx,
		URLPrefix: urlPrefix,
		Metas:     metas,
	}, template.HTMLEscapeString(text))
	if err != nil {
		log.Error("RenderIssueTitle: %v", err)
		return template.HTML("")
	}
	return template.HTML(renderedText)
}

// RenderEmoji renders html text with emoji post processors
func RenderEmoji(text string) template.HTML {
	renderedText, err := markup.RenderEmoji(template.HTMLEscapeString(text))
	if err != nil {
		log.Error("RenderEmoji: %v", err)
		return template.HTML("")
	}
	return template.HTML(renderedText)
}

// ReactionToEmoji renders emoji for use in reactions
func ReactionToEmoji(reaction string) template.HTML {
	val := emoji.FromCode(reaction)
	if val != nil {
		return template.HTML(val.Emoji)
	}
	val = emoji.FromAlias(reaction)
	if val != nil {
		return template.HTML(val.Emoji)
	}
	return template.HTML(fmt.Sprintf(`<img alt=":%s:" src="%s/assets/img/emoji/%s.png"></img>`, reaction, setting.StaticURLPrefix, url.PathEscape(reaction)))
}

// RenderNote renders the contents of a git-notes file as a commit message.
func RenderNote(ctx context.Context, msg, urlPrefix string, metas map[string]string) template.HTML {
	cleanMsg := template.HTMLEscapeString(msg)
	fullMessage, err := markup.RenderCommitMessage(&markup.RenderContext{
		Ctx:       ctx,
		URLPrefix: urlPrefix,
		Metas:     metas,
	}, cleanMsg)
	if err != nil {
		log.Error("RenderNote: %v", err)
		return ""
	}
	return template.HTML(fullMessage)
}

// IsMultilineCommitMessage checks to see if a commit message contains multiple lines.
func IsMultilineCommitMessage(msg string) bool {
	return strings.Count(strings.TrimSpace(msg), "\n") >= 1
}

// Actioner describes an action
type Actioner interface {
	GetOpType() activities_model.ActionType
	GetActUserName() string
	GetRepoUserName() string
	GetRepoName() string
	GetRepoPath() string
	GetRepoLink() string
	GetBranch() string
	GetContent() string
	GetCreate() time.Time
	GetIssueInfos() []string
}

// ActionIcon accepts an action operation type and returns an icon class name.
func ActionIcon(opType activities_model.ActionType) string {
	switch opType {
	case activities_model.ActionCreateRepo, activities_model.ActionTransferRepo, activities_model.ActionRenameRepo:
		return "repo"
	case activities_model.ActionCommitRepo, activities_model.ActionPushTag, activities_model.ActionDeleteTag, activities_model.ActionDeleteBranch:
		return "git-commit"
	case activities_model.ActionCreateIssue:
		return "issue-opened"
	case activities_model.ActionCreatePullRequest:
		return "git-pull-request"
	case activities_model.ActionCommentIssue, activities_model.ActionCommentPull:
		return "comment-discussion"
	case activities_model.ActionMergePullRequest, activities_model.ActionAutoMergePullRequest:
		return "git-merge"
	case activities_model.ActionCloseIssue, activities_model.ActionClosePullRequest:
		return "issue-closed"
	case activities_model.ActionReopenIssue, activities_model.ActionReopenPullRequest:
		return "issue-reopened"
	case activities_model.ActionMirrorSyncPush, activities_model.ActionMirrorSyncCreate, activities_model.ActionMirrorSyncDelete:
		return "mirror"
	case activities_model.ActionApprovePullRequest:
		return "check"
	case activities_model.ActionRejectPullRequest:
		return "diff"
	case activities_model.ActionPublishRelease:
		return "tag"
	case activities_model.ActionPullReviewDismissed:
		return "x"
	default:
		return "question"
	}
}

// ActionContent2Commits converts action content to push commits
func ActionContent2Commits(act Actioner) *repository.PushCommits {
	push := repository.NewPushCommits()

	if act == nil || act.GetContent() == "" {
		return push
	}

	if err := json.Unmarshal([]byte(act.GetContent()), push); err != nil {
		log.Error("json.Unmarshal:\n%s\nERROR: %v", act.GetContent(), err)
	}

	if push.Len == 0 {
		push.Len = len(push.Commits)
	}

	return push
}

// DiffTypeToStr returns diff type name
func DiffTypeToStr(diffType int) string {
	diffTypes := map[int]string{
		1: "add", 2: "modify", 3: "del", 4: "rename", 5: "copy",
	}
	return diffTypes[diffType]
}

// DiffLineTypeToStr returns diff line type name
func DiffLineTypeToStr(diffType int) string {
	switch diffType {
	case 2:
		return "add"
	case 3:
		return "del"
	case 4:
		return "tag"
	}
	return "same"
}

// MigrationIcon returns a SVG name matching the service an issue/comment was migrated from
func MigrationIcon(hostname string) string {
	switch hostname {
	case "github.com":
		return "octicon-mark-github"
	default:
		return "gitea-git"
	}
}

func buildSubjectBodyTemplate(stpl *texttmpl.Template, btpl *template.Template, name string, content []byte) {
	// Split template into subject and body
	var subjectContent []byte
	bodyContent := content
	loc := mailSubjectSplit.FindIndex(content)
	if loc != nil {
		subjectContent = content[0:loc[0]]
		bodyContent = content[loc[1]:]
	}
	if _, err := stpl.New(name).
		Parse(string(subjectContent)); err != nil {
		log.Warn("Failed to parse template [%s/subject]: %v", name, err)
	}
	if _, err := btpl.New(name).
		Parse(string(bodyContent)); err != nil {
		log.Warn("Failed to parse template [%s/body]: %v", name, err)
	}
}

type remoteAddress struct {
	Address  string
	Username string
	Password string
}

func mirrorRemoteAddress(ctx context.Context, m *repo_model.Repository, remoteName string, ignoreOriginalURL bool) remoteAddress {
	a := remoteAddress{}

	remoteURL := m.OriginalURL
	if ignoreOriginalURL || remoteURL == "" {
		var err error
		remoteURL, err = git.GetRemoteAddress(ctx, m.RepoPath(), remoteName)
		if err != nil {
			log.Error("GetRemoteURL %v", err)
			return a
		}
	}

	u, err := giturl.Parse(remoteURL)
	if err != nil {
		log.Error("giturl.Parse %v", err)
		return a
	}

	if u.Scheme != "ssh" && u.Scheme != "file" {
		if u.User != nil {
			a.Username = u.User.Username()
			a.Password, _ = u.User.Password()
		}
		u.User = nil
	}
	a.Address = u.String()

	return a
}

// JsPrettyNumber renders a number using english decimal separators, e.g. 1,200 and subsequent
// JS will replace the number with locale-specific separators, based on the user's selected language
func JsPrettyNumber(i interface{}) template.HTML {
	num := util.NumberIntoInt64(i)

	return template.HTML(`<span class="js-pretty-number" data-value="` + strconv.FormatInt(num, 10) + `">` + base.PrettyNumber(num) + `</span>`)
}
