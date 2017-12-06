# ircd-healthcheck-lambda

Uses [ircd-healthcheck][healthcheck] to check the status of IRC servers, and send Pushover
notifications on status changes.

## Setup

This assumes you have Apex installed and set up to connect to your AWS account.

* Create a DynamoDB table with the primary key `servers` (as a string), and
  add a boolean `isCurrentlyDown` field
* Copy `project.json` to `project.prod.json`
* Edit `project.prod.json` to have:
  * your DynamoDB table name
  * the servers you want to connect to (in the format below)
  * your Pushover application key and a comma-separated list of Pushover users to send notifications to
  * your IAM role
* Run `apex deploy --env prod`
* Set up a scheduler to run the `reporter` function however often you want the
  status to be checked

### Server list format

Entries are separated by `;`

* For plaintext: `server-one.example.com/6667`
* For SSL: `server-two.example.com/+6697`
* For SSL, without verifying the certificate is valid: `server-three.example.com/+?6697`

## Contributors

* [Alice Jenkinson (0x52a1)](https://github.com/0x52a1)

## License

MIT, see [LICENSE](./LICENSE)

[healthcheck]: https://github.com/stormbit/ircd-healthcheck


