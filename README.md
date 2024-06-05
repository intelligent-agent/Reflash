# Reflash

This is a simple Go server that is set up to
get and flash Refactor and Rebuild images

All users should download the latest stable version of
Reflash and use Balena Etcher to flash it to a USB drive. 

## Development
### Create linux image
This will use debootstrap to create a Reflash image
```
make image
```

### Local development
To start the npm client/Vue frontend
```
make dev-client
make run-go
```