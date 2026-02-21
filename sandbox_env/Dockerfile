# A lightweight python environment for testing code
FROM python:3.9-slim

WORKDIR /app

# Install standard testing tools
RUN pip install pytest

# We will mount the code to be tested at runtime
CMD ["pytest"]
