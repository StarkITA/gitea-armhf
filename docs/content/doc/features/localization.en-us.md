---
date: "2016-12-01T16:00:00+02:00"
title: "Localization"
slug: "localization"
weight: 10
toc: false
draft: false
menu:
  sidebar:
    parent: "features"
    name: "Localization"
    weight: 20
    identifier: "localization"
---

# Localization

Gitea's localization happens through our [Crowdin project](https://crowdin.com/project/gitea).

For changes to an **English** translation, a pull request can be made that changes the appropriate key in
the [english locale](https://github.com/go-gitea/gitea/blob/master/options/locale/locale_en-US.ini).

For changes to a **non-English** translation, refer to the Crowdin project above.

## Supported Languages

Any language listed in the above Crowdin project will be supported as long as 25% or more has been translated.

After a translation has been accepted, it will be reflected in the main repository after the next Crowdin sync, which is generally after any PR is merged.

At the time of writing, this means that a changed translation may not appear until the following Gitea release.

If you use a bleeding edge build, it should appear as soon as you update after the change is synced.
