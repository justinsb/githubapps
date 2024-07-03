group "default" {
  targets = ["codebot-githubapp-backend"]
}

target "codebot-githubapp-backend" {
  dockerfile = "images/codebot-githubapp-backend/Dockerfile"
}
