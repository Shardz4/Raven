# A lightweight python environment for testing code
FROM python:3.9-slim

WORKDIR /app

# Install standard testing tools and git for dynamic cloning
RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*
RUN pip install pytest

# We will mount the code to be tested at runtime
CMD ["pytest"]
