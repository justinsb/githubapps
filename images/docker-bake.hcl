group "default" {
  targets = ["codebot-githubapp-backend"]
}

variable "IMAGE_PREFIX" {
  default = ""
}

target "codebot-githubapp-backend" {
  dockerfile = "images/codebot-githubapp-backend/Dockerfile"
  tags = ["${IMAGE_PREFIX}codebot-githubapp-backend"]
}
