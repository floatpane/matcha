#!/usr/bin/env python3
"""Send a test email with quoted text to test the collapse feature."""
import smtplib
from email.mime.text import MIMEText
import getpass

EMAIL = "shabha2004@gmail.com"

body = """\
Hey, this is my reply!

On Mon, Jun 23, 2026 at 2:00 AM you@example.com wrote:
> This is the original message.
> It has multiple lines.
> Third line of quoted text.
> Fourth line of quoted text.
> Fifth line to make it obvious.
"""

msg = MIMEText(body)
msg["Subject"] = "Test quote collapse"
msg["From"] = EMAIL
msg["To"] = EMAIL

password = getpass.getpass("Enter Gmail App Password: ")

with smtplib.SMTP_SSL("smtp.gmail.com", 465) as server:
    server.login(EMAIL, password)
    server.send_message(msg)
    print("Email sent successfully!")
