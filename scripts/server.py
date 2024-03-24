#!/usr/bin/env python3

import http.server
import socketserver
import os

folder_path = os.environ.get("VORTEX_FILE_PATH", "/app/files")
port = int(os.environ.get("VORTEX_FILE_PORT", 18022))

Handler = http.server.SimpleHTTPRequestHandler

try:
    os.chdir(folder_path)
except FileNotFoundError:
    print(f"The specified folder '{folder_path}' does not exist.")
    exit()

with socketserver.TCPServer(("", port), Handler) as httpd:
    print(f"Serving at port {port}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nServer stopped.")
