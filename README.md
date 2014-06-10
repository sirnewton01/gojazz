# Go Jazz

GoJazz is a tool for working with Jazz SCM projects on IDS on your local machine.
It is designed to be a light-weight alternative to the standard Jazz SCM clients
that can work easily with a variety of desktop tools.

Working with Jazz SCM is accomplished via a minimal number of CLI commands and
 the IDS web interface for the rest of the SCM capabilities.

## Features

+  Load the latest contents of a stream (no authentication required for public projects)
+  Load the contents of your repository workspace
+  Check-in your changes (EXPERIMENTAL)
+  Incremental re-load, downloading only the changed files

## Examples

Load the default stream for a project. Repeat at any time to get the latest code.

`gojazz load "sirnewton | test"`

Load your repository workspace.

`gojazz load "sirnewton | test" -workspace=true -userId=mkent`

Find the modified files in your sandbox.

`gojazz status`

Check-in your changes to your repository workspace. Repeat whenever you have new changes.

`gojazz checkin`

## Updates

You can use IDS web interface is used to update your repository workspace.
The most common type of update is to accept changes from the team's stream.
There are other kinds of updates like discarding change sets or undoing changes.
Before you update your repository workspace you should check-in (gojazz checkin) your changes
to back them up. Afterwards, re-load (gojazz load) to update your sandbox.

