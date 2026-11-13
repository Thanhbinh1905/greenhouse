from datetime import datetime, timezone
from service import created
assert created() <= datetime.now(timezone.utc)
