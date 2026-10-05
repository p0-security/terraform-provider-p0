# The import ID is <integration>/<component>/<id>. The import leaves config unset, so
# the first apply after setting it re-sends it to P0 (and re-verifies the item).
terraform import p0_integration_item_staged.example aws/function-caller/arn:aws:lambda:us-west-2:123456789012:function:my-function
