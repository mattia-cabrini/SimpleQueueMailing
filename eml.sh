#!/bin/bash

print_header() {
	printf "%s: %s\r\n" "$1" "$2"
}

print_text() {
	printf "\r\n%s\r\n" "$1"
}

sep_multipart() {
	printf "\r\n--------------$1\r\n"
}

end_header_multipart() {
	printf "\r\n"
	printf "This is a multi-part message in MIME format."
	sep_multipart "$1"
}

text_multipart() {
	print_header Content-Type "$2; charset=UTF-8"
	print_header Content-Transfer-Encoding 7bit
	printf "\r\n%s\r\n" "$3"
	sep_multipart "$1"
}

# $1 Boundary
# $2 Mime-Type
# $3 Name
# $4 Path
attach_multipart() {
	print_header Content-Type "$2; charset=UTF-8; name=\"$3\""
	print_header Content-Disposition "attachment; filename=\"$3\""
	print_header Content-Transfer-Encoding base64
	printf "\r\n"
	base64 $4 | sed 's/$/\r/g'
	sep_multipart "$1"
}

case $1 in
	"header")
		print_header "$2" "$3"
		;;
	"text")
		print_text "$2"
		;;
	"attach")
		printf "attach\n"
		;;
	"set-multipart") # $2 boundary
		print_header Content-Type "multipart/mixed; boundary=\"------------$2\""
		print_header MIME-Version 1.0
		;;
	"end-header-multipart")
		end_header_multipart "$2"
		;;
	# "sep-multipart")
	# 	sep_multipart "$2"
	# 	;;
	"text-multipart")
		text_multipart "$2" "$3" "$4"
		;;
	"attach-multipart")
		attach_multipart "$2" "$3" "$4" "$5"
		;;
	*)
		printf "%s not supported\n" "$1"
		;;
esac
