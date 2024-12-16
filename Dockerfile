# Base Go image
FROM golang:1.20

# Install dependencies
RUN apt-get update && apt-get install -y ca-certificates && update-ca-certificates

# Set the working directory
WORKDIR /app

# sudo apt update && sudo apt install golang-go
# go mod init spork

# Copy Go modules and install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy application files
COPY . .

# check for main.go
RUN ls -l /app

# Expose the HTTP server port
EXPOSE 8080

# Run the application
CMD ["go", "run", "main.go"]
