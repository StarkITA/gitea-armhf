---
date: "2021-07-20T00:00:00+00:00"
title: "Maven Packages Repository"
slug: "packages/maven"
draft: false
toc: false
menu:
  sidebar:
    parent: "packages"
    name: "Maven"
    weight: 60
    identifier: "maven"
---

# Maven Packages Repository

Publish [Maven](https://maven.apache.org) packages for your user or organization.

**Table of Contents**

{{< toc >}}

## Requirements

To work with the Maven package registry, you can use [Maven](https://maven.apache.org/install.html) or [Gradle](https://gradle.org/install/).
The following examples use `Maven`.

## Configuring the package registry

To register the package registry you first need to add your access token to the [`settings.xml`](https://maven.apache.org/settings.html) file:

```xml
<settings>
  <servers>
    <server>
      <id>gitea</id>
      <configuration>
        <httpHeaders>
          <property>
            <name>Authorization</name>
            <value>token {access_token}</value>
          </property>
        </httpHeaders>
      </configuration>
    </server>
  </servers>
</settings>
```

Afterwards add the following sections to your project `pom.xml` file:

```xml
<repositories>
  <repository>
    <id>gitea</id>
    <url>https://gitea.example.com/api/packages/{owner}/maven</url>
  </repository>
</repositories>
<distributionManagement>
  <repository>
    <id>gitea</id>
    <url>https://gitea.example.com/api/packages/{owner}/maven</url>
  </repository>
  <snapshotRepository>
    <id>gitea</id>
    <url>https://gitea.example.com/api/packages/{owner}/maven</url>
  </snapshotRepository>
</distributionManagement>
```

| Parameter      | Description |
| -------------- | ----------- |
| `access_token` | Your [personal access token]({{< relref "doc/developers/api-usage.en-us.md#authentication" >}}). |
| `owner`        | The owner of the package. |

## Publish a package

To publish a package simply run:

```shell
mvn deploy
```

If you want to publish a prebuild package to the registry, you can use [`mvn deploy:deploy-file`](https://maven.apache.org/plugins/maven-deploy-plugin/deploy-file-mojo.html):

```shell
mvn deploy:deploy-file -Durl=https://gitea.example.com/api/packages/{owner}/maven -DrepositoryId=gitea -Dfile=/path/to/package.jar
```

| Parameter      | Description |
| -------------- | ----------- |
| `owner`        | The owner of the package. |

You cannot publish a package if a package of the same name and version already exists. You must delete the existing package first.

## Install a package

To install a Maven package from the package registry, add a new dependency to your project `pom.xml` file:

```xml
<dependency>
  <groupId>com.test.package</groupId>
  <artifactId>test_project</artifactId>
  <version>1.0.0</version>
</dependency>
```

Afterwards run:

```shell
mvn install
```

## Supported commands

```
mvn install
mvn deploy
mvn dependency:get:
```
