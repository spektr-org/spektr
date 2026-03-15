import requests

SPEKTR_URL = "https://your-spektr-endpoint"

csv = '''
Category,Field,Amount
Expense,Rent,2500
Expense,Groceries,800
Income,Salary,8000
'''

resp = requests.post(
    f"{SPEKTR_URL}/pipeline",
    json={
        "csv": csv,
        "query": "sum amount by category",
        "mode": "ai",
        "apiKey": "YOUR_API_KEY"
    }
).json()

print(resp["data"]["result"]["reply"])
