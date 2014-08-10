# Go Jazz

GoJazz is a tool for working with Jazz SCM projects on IBM DevOps Servies on your local machine.
It is designed to be a light-weight alternative to the standard Jazz SCM clients
so that you can work easily with your choice of editors and tools.

There are a minimal number of CLI commands for you to remember.
You can use the DevOps Services web site to use the rest of the SCM capabilities.

Gojazz should be suitable for many small to medium sized project. As a lightweight tool it lacks some of the more advanced "enterprise" features
of the standard Jazz SCM clients. Notable omissions include symbolic links,
platform-specific conversions of line terminators and the ability to script all of the SCM operations (e.g. accept, deliver). If it is not suitable for your
project please try one of Jazz SCM client from jazz.net/downloads.

## Features

+  Load the latest contents of a stream (no authentication required for public projects)
+  Load the contents of your personal repository workspace for projects
+  Synchronize your local changes with your repository workspace (EXPERIMENTAL)
+  Incremental load, downloading only the changed files in your stream or repository workspace

## Examples

Load the default stream for a project into a local sandbox. Repeat at any time to get the latest code.

`gojazz load "sirnewton | test"`

Load your repository workspace into a local sandbox. Repeat at any time to erase local changes and start over with what is in your repository workspace.

`gojazz load "sirnewton | test" -workspace=true -userId=mkent@example.com`

Find the modified files in your local sandbox.

`gojazz status`

Synchronize any local changes in your sandbox and changes in your repository workspace on the DevOps Services website.

`gojazz sync`

## Repository Workspaces

You have a repository workspace on IBM DevOps services to manage your
changes for a project before you share them with the rest of the team.
It's also a great place to backup your changes in case of disaster.

You can use DevOps Services web site to work with your repository workspaces.
The most common type of update is to accept changes from the team's stream.
There are other kinds of updates like discarding change sets or undoing changes.
Gojazz commands will give you the URL to access your repository workspace.

Run the "sync" command on your sandbox often. It will make sure that your
changes are backed up and it will make sure that you are up-to-date with
your repository workspace. 
