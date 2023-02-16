---
date: "2019-12-31T13:55:00+05:00"
title: "Advanced: Search Engines Indexation"
slug: "search-engines-indexation"
weight: 30
toc: false
draft: false
menu:
  sidebar:
    parent: "advanced"
    name: "Search Engines Indexation"
    weight: 60
    identifier: "search-engines-indexation"
---

# Search engines indexation of your Gitea installation

By default your Gitea installation will be indexed by search engines.
If you don't want your repository to be visible for search engines read further.

## Block search engines indexation using robots.txt

To make Gitea serve a custom `robots.txt` (default: empty 404) for top level installations,
create a file called `robots.txt` in the [`custom` folder or `CustomPath`]({{< relref "doc/advanced/customizing-gitea.en-us.md" >}})

Examples on how to configure the `robots.txt` can be found at [https://moz.com/learn/seo/robotstxt](https://moz.com/learn/seo/robotstxt).

```txt
User-agent: *
Disallow: /
```

If you installed Gitea in a subdirectory, you will need to create or edit the `robots.txt` in the top level directory.

```txt
User-agent: *
Disallow: /gitea/
```
