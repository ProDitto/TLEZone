import requests
import random
import string
import json
import time

API_URL = "http://localhost:8080/submit"
NUM_REQUESTS = 20  # change this to send more/less

def random_string(length=8):
    return ''.join(random.choices(string.ascii_letters + string.digits, k=length))

def send_task(task_id, name):
    payload = {
        "id": task_id,
        "name": name
    }
    try:
        response = requests.post(API_URL, json=payload)
        print(f"[{response.status_code}] {response.text.strip()}")
    except requests.exceptions.RequestException as e:
        print(f"[ERROR] {e}")

def main():
    time.sleep(3)
    for i in range(NUM_REQUESTS):
        task_id = random.randint(1000, 9999)
        name = f"task-{random_string(6)}"
        send_task(task_id, name)
        time.sleep(0.1)  # small delay between requests

if __name__ == "__main__":
    main()
