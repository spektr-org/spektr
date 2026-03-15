@echo off
echo Running Spektr demo queries...

echo.
echo Jira Example:
spektr --file datasets\jira_sample.csv --query "story points by assignee"

echo.
echo Finance Example:
spektr --file datasets\finance_sample.csv --query "expenses by category"

echo.
echo HR Example:
spektr --file datasets\hr_sample.csv --query "average salary by department"

echo.
echo Done.
