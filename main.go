package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gookit/color"
)

var verbose bool
var aggressive bool
var quick bool
var concurrency int

func main() {

	flag.BoolVar(&verbose, "v", false, "See more info on attempts")
	flag.BoolVar(&quick, "q", false, "Quick mode just checks the bucket ACL and for a directory listing. No enumeration of objects")
	flag.BoolVar(&aggressive, "a", false, "Be aggressive and attempt to write to the bucket/object policy")
	flag.IntVar(&concurrency, "c", 10, "Set the concurrency level, default 10")

	flag.Parse()

	ctx := context.TODO()
	scanner := bufio.NewScanner(os.Stdin)

	// Check if stdin is connected to a terminal or a pipe/file
	fileInfo, _ := os.Stdin.Stat()
	if (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		fmt.Println("No input detected. Please provide a list of bucket names via stdin.")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	bucketsChan := make(chan string)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for bucketName := range bucketsChan {
				processBucket(ctx, bucketName)
			}
		}()
	}

	// Read bucket names from stdin and send them to the channel
	for scanner.Scan() {
		bucketName := scanner.Text()
		bucketsChan <- bucketName
	}
	close(bucketsChan)

	wg.Wait()

}

func processBucket(ctx context.Context, bucketName string) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Unable to load SDK config, %v", err)
	}

	bucketRegion, err := getBucketRegion(bucketName)
	if err != nil {
		if verbose {
			fmt.Printf("Unable to get the region for %s\n", bucketName)
		}
		return
	}
	if verbose {
		fmt.Printf("Bucket %s found in Region %s\n", bucketName, bucketRegion)
	}

	cfg.Region = bucketRegion
	client := s3.NewFromConfig(cfg)

	checkBucketACL(ctx, client, bucketName)
	checkOpenListing(ctx, client, bucketName)

	if quick {
		return
	}

	if aggressive {
		testUpload(ctx, client, bucketName, "s3-warden-test.txt", strings.NewReader("s3-warden-test"))
		putBucketACP(ctx, client, bucketName)
	}

	iterateBucket(ctx, client, bucketName)
}

func getBucketRegion(bucket string) (string, error) {
	url := fmt.Sprintf("https://%s.s3.amazonaws.com", bucket)

	// Create a custom HTTP client that ignores SSL certificate errors
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	client := &http.Client{Transport: customTransport}

	resp, err := client.Head(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	region := resp.Header.Get("x-amz-bucket-region")
	if region == "" {
		return "", fmt.Errorf("bucket region not found in headers")
	}
	return region, nil
}

func checkOpenListing(ctx context.Context, client *s3.Client, bucket string) {
	_, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1),
	})

	if err != nil {
		if verbose {
			fmt.Printf("No open directory listing found in: %s\n", bucket)
		}
	} else {
		if verbose {
			color.Yellow.Printf("Possible open directory listing in %s\n", bucket)
		} else {
			fmt.Printf("Possible open directory listing in %s\n", bucket)
		}
	}
}

func checkBucketACL(ctx context.Context, client *s3.Client, bucket string) {
	aclOutput, err := client.GetBucketAcl(ctx, &s3.GetBucketAclInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		if verbose {
			fmt.Printf("Failed to get ACL for bucket %s\n", bucket)
		}
		return
	}

	hasPublicRead := false
	hasPublicWrite := false

	for _, grant := range aclOutput.Grants {
		if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
			switch grant.Permission {
			case types.PermissionRead:
				hasPublicRead = true
			case types.PermissionWrite, types.PermissionFullControl:
				hasPublicWrite = true
			}
		}
	}

	// Decide what to print based on the flags
	if hasPublicWrite {
		if verbose {
			color.Red.Printf("Bucket with public write access found: %s\n", bucket)
		} else {
			fmt.Printf("Bucket with public write access found: %s\n", bucket)
		}
	}

	if hasPublicRead {
		if verbose {
			color.Yellow.Printf("Bucket with public read access found: %s\n", bucket)
		} else {
			fmt.Printf("Bucket with public read access found: %s\n", bucket)
		}
	}

	if verbose && !hasPublicRead && !hasPublicWrite {
		fmt.Printf("No public access found on bucket %s\n", bucket)
	}

	return
}

func testUpload(ctx context.Context, client *s3.Client, bucket string, key string, body *strings.Reader) {
	if verbose {
		fmt.Printf("Attempting to upload file to %s\n", bucket)
	}
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return
	}
	if verbose {
		color.Green.Printf("Upload allowed in bucket %s\n", bucket)
	} else {
		fmt.Printf("Upload allowed in bucket %s\n", bucket)
	}
	return
}

func putBucketACP(ctx context.Context, client *s3.Client, bucket string) {
	if verbose {
		fmt.Printf("Attempting to write bucket ACP to %s\n", bucket)
	}
	_, err := client.PutBucketAcl(ctx, &s3.PutBucketAclInput{
		Bucket:    aws.String(bucket),
		GrantRead: aws.String("uri=http://acs.amazonaws.com/groups/global/AuthenticatedUsers"),
	})
	if err != nil {
		return
	}
	if verbose {
		color.Green.Printf("Writable Bucket ACP in bucket %s\n", bucket)
	} else {
		fmt.Printf("Writable Bucket ACP in bucket %s\n", bucket)
	}
	return
}

func putObjectACP(ctx context.Context, client *s3.Client, bucket string, key string) {
	if verbose {
		fmt.Printf("Attempting to write object ACP to %s/%s\n", bucket, key)
	}
	_, err := client.PutObjectAcl(ctx, &s3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    "public-read",
	})
	if err != nil {
		if verbose {
			fmt.Printf("Failed to write object ACP to %s/%s\n", bucket, key)
		}
		return
	}
	if verbose {
		color.Green.Printf("Writable Bucket Object ACP %s/%s\n", bucket, key)
	} else {
		fmt.Printf("Writable Bucket Object ACP %s/%s\n", bucket, key)
	}
}

func iterateBucket(ctx context.Context, client *s3.Client, bucket string) {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})

	// if 5 issues are found, it's enough to stop and move on
	issueCounter := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			if verbose {
				fmt.Printf("Failed to iterate page in bucket %s\n", bucket)
			}
			break
		}

		for _, object := range page.Contents {
			if aggressive {
				putObjectACP(ctx, client, bucket, *object.Key)
			}
			if verbose {
				fmt.Printf("Checking ACP on %s/%s\n", bucket, *object.Key)
			}

			// Get the ACL for each object
			aclOutput, err := client.GetObjectAcl(ctx, &s3.GetObjectAclInput{
				Bucket: aws.String(bucket),
				Key:    object.Key,
			})
			if err != nil {
				if verbose {
					fmt.Printf("Failed to get ACL for object %s/%s\n", bucket, *object.Key)
				}
				continue
			}

			// Check if the ACL includes permissions by unauthorized users
			// Initialize flags to track found permissions
			hasPublicRead := false
			hasPublicWrite := false

			for _, grant := range aclOutput.Grants {
				if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
					switch grant.Permission {
					case types.PermissionRead:
						hasPublicRead = true
					case types.PermissionWrite, types.PermissionFullControl:
						hasPublicWrite = true
					}
				}
			}

			// Decide what to print based on the flags
			if hasPublicWrite {
				if verbose {
					color.Red.Printf("Object with public write access found: %s/%s\n", bucket, *object.Key)
				} else {
					fmt.Printf("Object with public write access found: %s/%s\n", bucket, *object.Key)
				}
				issueCounter++
				if issueCounter >= 5 {
					if verbose {
						fmt.Printf("Found 5 objects with public write permissions in %s, skipping the rest.\n", bucket)
					}
					return
				}
			}

			if hasPublicRead {
				if verbose {
					color.Yellow.Printf("Object with public read access found: %s/%s\n", bucket, *object.Key)
				} else {
					fmt.Printf("Object with public read access found: %s/%s\n", bucket, *object.Key)
				}
			}

		}
	}
}
