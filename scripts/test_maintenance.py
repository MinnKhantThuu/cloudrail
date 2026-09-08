import copy
import pathlib
import tempfile
import unittest
from unittest.mock import patch
import maintenance


class MaintenanceGuards(unittest.TestCase):
    def setUp(self):
        self.identity = {'dockerID': 'same-host', 'environmentSHA256': 'same-installation'}
        self.schema = {'migrations': [{'name': '001.sql', 'checksum': 'aaa'}], 'ddlSHA256': 'schema'}
        self.record = {'format': 1, 'backupComplete': True, 'identity': self.identity,
                       'schema': self.schema, 'images': {role: {'id': 'sha256:' + 'a' * 64,
                       'ref': 'cloudrail-' + role + ':previous-test'} for role in maintenance.ROLES}}

    def test_compatible_record_accepted(self):
        maintenance.validate_rollback(self.record, self.identity, self.schema)

    def test_other_host_or_environment_rejected(self):
        for field in self.identity:
            altered = dict(self.identity, **{field: 'changed'})
            with self.assertRaisesRegex(RuntimeError, 'host or installation'):
                maintenance.validate_rollback(self.record, altered, self.schema)

    def test_schema_or_ledger_changes_rejected(self):
        for field in self.schema:
            altered = copy.deepcopy(self.schema)
            altered[field] = [] if field == 'migrations' else 'changed'
            with self.assertRaisesRegex(RuntimeError, 'schema changed'):
                maintenance.validate_rollback(self.record, self.identity, altered)

    def test_unfinished_backup_rejected(self):
        self.record['backupComplete'] = False
        with self.assertRaisesRegex(RuntimeError, 'completed maintenance backup'):
            maintenance.validate_rollback(self.record, self.identity, self.schema)

    def test_arbitrary_recovery_image_rejected(self):
        self.record['images']['server']['ref'] = 'other/application:latest'
        with self.assertRaisesRegex(RuntimeError, 'recovery tag'):
            maintenance.validate_rollback(self.record, self.identity, self.schema)

    def test_operators_cannot_overlap_and_lock_releases(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(maintenance, 'ROOT', pathlib.Path(temporary)):
            with maintenance.maintenance_lock():
                with self.assertRaisesRegex(RuntimeError, 'already running'):
                    with maintenance.maintenance_lock():
                        self.fail('Second operator acquired lock')
            with maintenance.maintenance_lock():
                pass


if __name__ == '__main__':
    unittest.main()
