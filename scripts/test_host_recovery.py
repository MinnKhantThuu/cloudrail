import copy
import importlib.util
import io
import json
import pathlib
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('host_recovery', pathlib.Path(__file__).with_name('host-recovery.py'))
h = importlib.util.module_from_spec(spec); spec.loader.exec_module(h)


class HostRecoveryBoundaries(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)

    def archive(self, members):
        target = self.root / 'volume.tar'
        with tarfile.open(target, 'w') as tar:
            for name, kind, link in members:
                info = tarfile.TarInfo(name); info.type = kind; info.linkname = link
                info.size = 3 if kind == tarfile.REGTYPE else 0
                tar.addfile(info, io.BytesIO(b'abc') if info.size else None)
        return target

    def test_regular_files_and_internal_links_preserved(self):
        path = self.archive([('./data',tarfile.REGTYPE,''),('./alias',tarfile.SYMTYPE,'data'),('./hard',tarfile.LNKTYPE,'./data')])
        self.assertEqual(h.volume_archive(path),3)

    def test_archive_escape_special_files_and_symlink_writes_rejected(self):
        for members in [
            [('../escape',tarfile.REGTYPE,'')], [('/absolute',tarfile.REGTYPE,'')],
            [('link',tarfile.SYMTYPE,'../../escape')], [('link',tarfile.SYMTYPE,'/etc')],
            [('link',tarfile.SYMTYPE,'folder'),('link/write',tarfile.REGTYPE,'')],
            [('pipe',tarfile.FIFOTYPE,'')], [('hard',tarfile.LNKTYPE,'absent')],
            [('duplicate',tarfile.REGTYPE,''),('duplicate',tarfile.REGTYPE,'')],
        ]:
            with self.subTest(members=members), self.assertRaises(RuntimeError):
                h.volume_archive(self.archive(members))

    def record(self):
        identifier='a'*32
        names=['cloudrail_'+n for n in sorted(h.PERSISTENT-{'certificates'})]
        record={'format':1,'complete':True,'id':identifier,'public':False,'host':{'id':'source','architecture':'x86_64'},
                'volumes':[{'name':name,'labels':{}} for name in names], 'applicationImages':[],
                'images':{role:{'id':'sha256:'+'b'*64,'ref':'cloudrail-host-'+role+':'+identifier} for role in h.ROLES}, 'files':{}}
        data=self.archive([('proof',tarfile.REGTYPE,'')]).read_bytes()
        for name in names:
            (self.root/(name+'.tar')).write_bytes(data)
        for name in ('installation.env','platform-images.tar'):
            (self.root/name).write_text('fixture')
        for name in [n+'.tar' for n in names]+['installation.env','platform-images.tar']:
            path=self.root/name;record['files'][name]={'sha256':h.checksum(path),'bytes':path.stat().st_size}
        (self.root/'manifest.json').write_text(json.dumps(record))
        return record

    def test_incomplete_corrupt_missing_and_unexpected_payloads_rejected(self):
        record=self.record();h.validate(self.root)
        for mutate in [lambda r:r.update(complete=False), lambda r:r['files'].pop('installation.env'),
                       lambda r:r['volumes'].append(r['volumes'][0]), lambda r:r.update(public=True),
                       lambda r:r['images']['agent'].update(ref='unrelated:latest')]:
            changed=copy.deepcopy(record);mutate(changed)
            (self.root/'manifest.json').write_text(json.dumps(changed))
            with self.assertRaises(RuntimeError):h.validate(self.root)
        (self.root/'manifest.json').write_text(json.dumps(record))
        (self.root/'installation.env').write_text('tampered')
        with self.assertRaisesRegex(RuntimeError,'checksum'):h.validate(self.root)

    def test_fencing_and_source_host_rejection_precede_mutation(self):
        with patch.object(h,'docker') as docker:
            with self.assertRaisesRegex(RuntimeError,'Fence'):h.restore(self.root,False,None)
            docker.assert_not_called()
        record=self.record()
        with patch.object(h,'host',return_value=record['host']),patch.object(h,'docker') as docker:
            with self.assertRaisesRegex(RuntimeError,'different Docker host'):h.restore(self.root,True,None)
            docker.assert_not_called()

    def test_existing_installation_rejected_before_docker_load(self):
        record=self.record();record.update(version='test',config={})
        (self.root/'manifest.json').write_text(json.dumps(record))
        (self.root/'VERSION').write_text('test');(self.root/'deploy/local').mkdir(parents=True)
        (self.root/'deploy/local/.env').write_text('existing')
        with patch.object(h,'ROOT',self.root),patch.object(h,'config_hashes',return_value={}),patch.object(h,'host',return_value={'id':'other','architecture':'x86_64'}),patch.object(h,'docker') as docker:
            with self.assertRaisesRegex(RuntimeError,'empty Docker host'):h.restore(self.root,True,None)
            docker.assert_not_called()


if __name__=='__main__':unittest.main()
