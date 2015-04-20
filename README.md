#Ninja Sphere GoLang Lifx Driver

##Building
Run `make` in the directory of the driver

or to develop on mac and run on the sphere
`GOOS=linux GOARCH=arm go build -o lifx main.go driver.go version.go && scp lifx ninja@ninjasphere.local:~/`

##Running
Run `./bin/driver-go-lifx` from the `bin` directory after building 

# Licensing

driver-go-lifx is licensed under the MIT License. See LICENSE for the full license text.
