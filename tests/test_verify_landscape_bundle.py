from mock import (
    patch,
)

import tests
from tests.test_jujupy import FakeJujuClient
from verify_landscape_bundle import(
    assess_landscape_bundle,
)


class TestVerifyLandscapeBundle(tests.TestCase):

    def test_assert_landscape_bundle(self):
        client = FakeJujuClient()
        services = ['haproxy', 'landscape-server', 'postgresql',
                    'rabbitmq-server']
        with patch('verify_landscape_bundle.verify_services',
                   autospec=True) as vs_mock:
            assess_landscape_bundle(client)
        vs_mock.assert_called_once_with(client, services, scheme='https',
                                        text='Landscape')
