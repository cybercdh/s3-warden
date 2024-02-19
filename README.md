# s3-warden

s3-warden is a powerful tool designed to help AWS users audit and monitor the Access Control Policies (ACPs) of their S3 buckets and objects. It provides a simple yet effective way to identify open or public ACPs that could potentially expose sensitive data. With s3-warden, you can ensure that your S3 buckets and objects are secured according to best practices, protecting your data from unauthorized access.

## Features

- **Bucket ACP Auditing**: Quickly check if your S3 bucket's ACP configuration allows public access.
- **Object ACP Inspection**: Drill down into individual objects within a bucket to assess their ACP settings.
- **Region Discovery**: Automatically determines the bucket's region to perform accurate and efficient ACP checks.
- **Verbose Output**: Option to get detailed information about the ACP checks being performed, enhancing transparency and debuggability.

## Getting Started

### Prerequisites

- AWS CLI configured with appropriate permissions
- Go 1.x or later

### Installation

Clone the repository to your local machine:

```sh
git clone https://github.com/cybercdh/s3-warden.git
cd s3-warden
go build -o s3-warden main.go
```

or install the latest version

```sh
go install https://github.com/cybercdh/s3-warden@latest
```

### Usage
To use s3-warden, simply provide the bucket name using the -b flag and optionally enable verbose output with -v:

```sh
Usage of s3-warden:
  -a	Be aggressive and attempt to write to the bucket/object policy
  -c int
    	Set the concurrency level, default 10 (default 10)
  -v	See more info on attempts
```
Note: Ensure that your AWS CLI is configured with the necessary permissions to fetch bucket and object ACLs.

## Contributing

Pull requests are welcome. For major changes, please open an issue first
to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License

[MIT](https://choosealicense.com/licenses/mit/)