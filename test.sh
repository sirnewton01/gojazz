#/bin/bash

if [ "$DOS_USERID" = "" ]; then
	read -p "User ID:" DOS_USERID
	export DOS_USERID
fi

if [ "$DOS_PASSWORD_FILE" = "" ]; then
	echo "Your password will be stored in a file here called password.txt protected with OS permissions"
	read -s -p "Password:" DOS_PASSWORD
	echo $DOS_PASSWORD > password.txt
	chmod 600 ./password.txt
	export DOS_PASSWORD_FILE=password.txt
fi

go test -test.v
