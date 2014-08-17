#/bin/bash

if [ ! -f $HOME/.gojazz/credentials.txt ]; then
	read -p "User ID: " DOS_USERID
	read -s -p "Password: " DOS_PASSWORD
	mkdir $HOME/.gojazz
	chmod 0700 $HOME/.gojazz
	echo "" > $HOME/.gojazz/credentials.txt
	chmod 0600 $HOME/.gojazz/credentials.txt
	echo $DOS_USERID > $HOME/.gojazz/credentials.txt
	echo $DOS_PASSWORD >> $HOME/.gojazz/credentials.txt
fi

go test -test.v
