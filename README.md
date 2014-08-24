# Go Jazz

GoJazz is a tool for working with Jazz SCM projects on IBM DevOps Servies on your local machine.
It is designed to be a light-weight alternative to the standard Jazz SCM clients
so that you can work easily with your choice of editors and tools.

There are a minimal number of CLI commands for you to remember.
You can use the DevOps Services web site to use the rest of the SCM and manage your project.

Gojazz should be suitable for many small to medium sized project. As a lightweight tool it lacks some of the more advanced "enterprise" features
of the standard Jazz SCM clients. Notable omissions include symbolic links,
platform-specific conversions of line terminators and the ability to script all of the SCM operations (e.g. accept, deliver). If it is not suitable for your
project please try one of the official Jazz SCM client from jazz.net/downloads.

Check out the youtube videos:
* [Intro](https://www.youtube.com/watch?v=8YVGOBX2--E)

## Features

+  Load the latest contents of a stream (no authentication required for public projects)
+  Load the contents of your personal repository workspace for projects
+  Synchronize your local changes with your repository workspace (EXPERIMENTAL)
+  Incremental load, downloading only the changed files in your stream or repository workspace
+  Build/test your code while automatically uploading the results to your project (EXPERIMENTAL)

## Examples

Load the default stream for a project into a local sandbox. Repeat at any time to get the latest code.

`gojazz load "sirnewton | test"`

Load your repository workspace into a local sandbox. Repeat at any time to erase local changes and start over with what is in your repository workspace.

`gojazz load "sirnewton | test" -workspace=true`

Find the modified files in your local sandbox.

`gojazz status`

Synchronize any local changes in your sandbox and changes in your repository workspace on the DevOps Services website.

`gojazz sync`

## Repository Workspaces

You have a repository workspace on IBM DevOps services to manage your
changes for a project before you share them with the rest of the team.
It hooks into the rest of the project management capabilities of the website
such as tracking and planning.
It's also a great place to backup your changes in case of disaster.

You can use DevOps Services web site to work with your repository workspaces.
The most common type of update is to accept changes from the team's stream.
There are other kinds of updates like discarding change sets or undoing changes.
Gojazz commands will give you the URL to access your repository workspace.

Run the "sync" command on your sandbox often. It will make sure that your
changes are backed up and it will make sure that you are up-to-date with
your repository workspace. As a rule of thumb, you should sync whenever you make changes to your sandbox or when you make changes to your repository workspace on the website.

## Build

Gojazz also helps you to record the results of automated builds. Once you have loaded a stream into a sandbox you can use the build command to run your regular build tool and upload the status and log to your project on IBM DevOps Services.It's best to use a separate sandbox, account or even VM to run your automated build.

`gojazz load "sirnewton | test"`

`gojazz build -- make`

The load command will download the stream for the test project into a sandbox. The build command will automatically update that sandbox and run the command provided after the "--." This is the normal command-line that you would use to build and/or test your code (e.g. make, maven). If the command returns a non-zero value then the build is considered to be an error, otherwise it is a pass. All output from running the command is captured and stored in the result stored on IBM DevOps Services. Once the process is complete Gojazz will give you the URL to access your build results.

To run a build on a schedule you can put the Gojazz build command in a cron job or similar.

## Supported Platforms

Linux, Mac OS

## Downloads

You can [download](https://hub.jazz.net/ccm04/web/projects/sirnewton%20%7C%20gojazz#action=com.ibm.team.build.viewDefinition&id=_VwuOYL_IvO21rKYNjWNf8Q) the latest build of gojazz for Mac (aka. Darwin) and Linux 32-bit and 64-bit (amd64).

